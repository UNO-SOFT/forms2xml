// Copyright 2019 Tamás Gulácsi
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

//go:generate env OHOME=/oracle/mw11gR1/fr11gR2 sh -c "set -x; javac -cp classes:${DOLLAR}OHOME/jlib/frmjdapi.jar:${DOLLAR}OHOME/jlib/frmxmltools.jar:${DOLLAR}OHOME/lib/xmlparserv2.jar -d classes src/unosoft/forms/Serve.java"

//go:generate statik  -m -f -p statik -src classes

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	"github.com/rjeczalik/notify"
	"github.com/tgulacsi/go/httpclient"
	"github.com/tgulacsi/go/iohlp"
	"golang.org/x/sync/errgroup"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/UNO-SOFT/forms2xml/transform"

	_ "github.com/UNO-SOFT/forms2xml/statik"
	"github.com/rakyll/statik/fs"
)

func main() {
	if err := Main(); err != nil {
		log.Fatalf("%+v", err)
	}
}

var jdapiURLs = []string{os.Getenv("BRUNO_ID")}

func Main() error {
	if len(jdapiURLs) == 1 {
		jdapiURLs = append(jdapiURLs, jdapiURLs[0])
	}

	var concurrency = 8
	formsLibPath, display := os.Getenv("FORMS_PATH"), os.Getenv("DISPLAY")
	if formsLibPath == "" {
		formsLibPath = filepath.Join(filepath.Dir(os.Getenv("BRUNO_HOME")), "lib")
	}
	app := kingpin.New("forms2xml", "Oracle Forms .fmb <-> .xml with optional conversion")
	app.Flag("jdapi-src", "SRC Form JDAPI helper HTTP listener URL").
		Default(jdapiURLs[0]).StringVar(&jdapiURLs[0])
	app.Flag("jdapi-dst", "DEST Form JDAPI helper HTTP listener URL").
		Default(jdapiURLs[1]).StringVar(&jdapiURLs[1])
	app.Flag("forms.lib.path", "FORMS_PATH").Default(formsLibPath).StringVar(&formsLibPath)
	app.Flag("display", "DISPLAY").Default(display).StringVar(&display)

	cmdXML := app.Command("xml", "convert to-from XML").Default()
	xmlSrc := cmdXML.Arg("src", "source file").ExistingFile()
	xmlDst := cmdXML.Arg("dst", "destination file").String()

	cmdTransform := app.Command("transform", "transform the XML")
	tranSrc := cmdTransform.Arg("src", "source file").ExistingFile()
	tranDst := cmdTransform.Arg("dst", "destination file").String()

	fileSuffix := "-v11"
	cmd6211 := app.Command("6to11", "convert from Forms v6 to v11").Alias("6211").Alias("convert")
	upNoTransform := cmd6211.Flag("no-transform", "don't transform").Default("false").Bool()
	upSrc := cmd6211.Arg("src", "source file").ExistingFile()
	upDst := cmd6211.Arg("dst", "destination file").String()
	upSuffix := cmd6211.Flag("suffix", "suffix of converted files").Default(fileSuffix).String()

	var watchSrc, watchDst string
	cmdWatch := app.Command("watch", "watch a directory and transform all appearing files")
	cmdWatch.Arg("src", "source path to watch").ExistingDirVar(&watchSrc)
	cmdWatch.Arg("dst", "destination path").ExistingDirVar(&watchDst)
	cmdWatch.Flag("suffix", "suffix of converted files").Default(fileSuffix).StringVar(&fileSuffix)
	watchNoTransform := cmdWatch.Flag("no-transform", "don't transform").Default("false").Bool()
	cmdWatch.Flag("concurrency", "maximum number of conversions running in parallel").Default(strconv.Itoa(concurrency)).IntVar(&concurrency)

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	go func() {
		<-sigCh
		cancel()
		time.Sleep(time.Second)
		os.Exit(1)
	}()
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var converter Converter
	if strings.Contains(jdapiURLs[0], "@") || strings.Contains(jdapiURLs[0], "/") && !strings.HasPrefix(jdapiURLs[0], "http") {
		converter = &javaRunner{DbConn: jdapiURLs[0], FormsLibPath: formsLibPath, Display: display,
			globalCtx: ctx}
		defer (converter.(io.Closer)).Close()
	} else {
		converter = newHTTPClient(jdapiURLs)
	}
	log.Println("converter:", converter)

	switch cmd {
	case cmdXML.FullCommand():
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := convertFiles(ctx, converter, *xmlDst, *xmlSrc)
		cancel()
		return err

	case cmdTransform.FullCommand():
		return transformFiles(*tranDst, *tranSrc)

	case cmd6211.FullCommand():
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := convertFiles6to11(ctx, converter, *upDst, *upSrc, !*upNoTransform, *upSuffix)
		cancel()
		return err

	case cmdWatch.FullCommand():
		return watchConvert(ctx, converter, watchDst, watchSrc, !*watchNoTransform, fileSuffix, concurrency)
	}
	return nil
}

func watchConvert(ctx context.Context, converter Converter, dstDir, srcDir string, doTransform bool, suffix string, concurrency int) error {
	tokens := make(chan struct{}, concurrency)
	eventCh := make(chan notify.EventInfo, 16)
	if err := notify.Watch(srcDir, eventCh, eventsToWatch...); err != nil {
		return errors.Wrap(err, "watch")
	}
	for evt := range eventCh {
		fn := evt.Path()
		bn := filepath.Base(fn)
		if !strings.HasSuffix(bn, ".fmb") {
			continue
		}
		go func() {
			time.Sleep(time.Second)
			tokens <- struct{}{}
			defer func() { <-tokens }()
			for i := 0; i < 10; i++ {
				err := convertFiles6to11(
					ctx, converter,
					filepath.Join(dstDir, bn), fn, doTransform, suffix,
				)
				if err == nil {
					break
				}
				log.Println(err)
				time.Sleep(time.Duration(i) * time.Second)
			}
		}()
	}
	return nil
}

func transformFiles(dst, src string) error {
	inp := os.Stdin
	if !(src == "" || src == "-") {
		var err error
		if inp, err = os.Open(src); err != nil {
			return errors.Wrap(err, "open "+src)
		}
	}
	defer inp.Close()

	out := os.Stdout
	if !(dst == "" || dst == "-") {
		var err error
		if out, err = os.Create(dst); err != nil {
			return errors.Wrap(err, "create "+dst)
		}
	}
	defer out.Close()

	var P transform.FormsXMLProcessor
	if err := P.ProcessStream(out, inp); err != nil {
		return errors.WithMessage(err, "processStream")
	}
	return out.Close()
}

func convertFiles6to11(ctx context.Context, converter Converter, dst, src string, doTransform bool, suffix string) error {
	if dst == "" {
		dst = strings.TrimSuffix(src, ".fmb") + suffix + ".fmb"
	}
	if dst == src {
		return errors.Wrap(errors.New("overwrite source file"), src)
	}
	log.Printf("Convert %q to %q.", src, dst)
	inp, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "open "+src)
	}
	defer inp.Close()
	if dfi, err := os.Stat(dst); err == nil {
		sfi, err := inp.Stat()
		if err != nil {
			return errors.Wrap(err, "stat "+dst)
		}
		if os.SameFile(sfi, dfi) {
			return errors.Wrap(errors.New("overwrite source file"), sfi.Name())
		}
	}

	out, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "create "+dst)
	}
	defer out.Close()
	xr, xw := io.Pipe()
	var P transform.FormsXMLProcessor
	var grp errgroup.Group
	grp.Go(func() error {
		xmlSource := xr
		if doTransform {
			xmlR := io.ReadCloser(xr)
			tr, tw := io.Pipe()
			xmlW := io.WriteCloser(tw)
			xmlSrcFn := strings.TrimSuffix(src, ".fmb") + ".xml"
			if xmlSrcFh, err := os.Create(xmlSrcFn); err != nil {
				log.Println(err)
			} else {
				defer xmlSrcFh.Close()
				xmlR = struct {
					io.Reader
					io.Closer
				}{io.TeeReader(xr, xmlSrcFh), xr}
			}
			xmlFn := strings.TrimSuffix(dst, ".fmb") + ".xml"
			if xmlFh, err := os.Create(xmlFn); err != nil {
				log.Println(err)
			} else {
				defer xmlFh.Close()
				xmlW = struct {
					io.Writer
					io.Closer
				}{io.MultiWriter(tw, xmlFh), tw}
			}
			xmlSource = tr
			grp.Go(func() error {
				log.Println("start transform")
				err := P.ProcessStream(xmlW, xmlR)
				log.Printf("xml->xml: %+v", err)
				tw.CloseWithError(err)
				return errors.WithMessage(err, "processStream")
			})
		}
		log.Println("start convert")
		err := converter.Convert(ctx, out, xmlSource, "application/xml")
		log.Printf("xml->fmb: %+v", err)
		xr.CloseWithError(err)
		return errors.WithMessage(err, "convert")
	})
	err = converter.Convert(ctx, xw, inp, "application/x-oracle-forms")
	log.Printf("fmb->xml: %+v", err)
	xw.CloseWithError(err)
	if err != nil {
		return errors.WithMessage(err, "convert")
	}
	if err = grp.Wait(); err != nil {
		return errors.WithMessage(err, "convertFiles6to11")
	}
	return out.Close()
}

func convertFiles(ctx context.Context, converter Converter, dst, src string) error {
	mimeType := "application/x-oracle-forms"
	inp := io.ReadCloser(os.Stdin)
	var err error
	if src != "" && src != "-" {
		return converter.ConvertFiles(ctx, dst, src)
	} else {
		var a [1024]byte
		n, err := io.ReadAtLeast(inp, a[:], 4)
		if err != nil {
			return errors.Wrap(err, "readAtLeast stdin")
		}
		if bytes.HasPrefix(bytes.TrimSpace(a[:n]), []byte("<?xml")) {
			mimeType = "application/xml"
		}
		inp = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(a[:n]), inp), inp}
	}
	defer inp.Close()

	log.Println(src, mimeType)

	out := os.Stdout
	if dst != "" && dst != "-" {
		if out, err = os.Create(dst); err != nil {
			return errors.Wrap(err, "create "+dst)
		}
		defer out.Close()
	}

	if err = converter.Convert(ctx, out, inp, mimeType); err != nil {
		return errors.WithMessage(err, "convertFiles")
	}
	return out.Close()
}

type javaRunner struct {
	DbConn, Display, FormsLibPath  string
	mu                             sync.Mutex
	classes, classpath, oracleHome string

	globalCtx context.Context
	cancel    context.CancelFunc
	request   io.Writer
	answer    *bufio.Scanner
}

func (jr *javaRunner) Close() error {
	jr.mu.Lock()
	err := jr.stop()
	jr.mu.Unlock()
	return errors.WithMessage(err, "Close")
}

func (jr *javaRunner) stop() error {
	cancel, classes := jr.cancel, jr.classes
	jr.classes, jr.classpath = "", ""
	jr.cancel, jr.request, jr.answer = nil, nil, nil
	if cancel != nil {
		cancel()
	}
	if classes != "" {
		os.RemoveAll(classes)
	}
	return nil
}

func (jr *javaRunner) start() error {
	if jr.classpath == "" {
		statikFS, err := fs.New()
		if err != nil {
			return errors.Wrap(err, "open statik fs")
		}
		if jr.classes, err = ioutil.TempDir("", "forms2xml-classes-"); err != nil {
			if err != nil {
				return errors.Wrap(err, "create temp dir for classes")
			}
		}

		for _, fn := range []string{"/unosoft/forms/Serve$ConvertHandler.class", "/unosoft/forms/Serve.class"} {
			b, err := fs.ReadFile(statikFS, fn)
			if err != nil {
				return errors.Wrap(err, "read "+fn)
			}
			fn = filepath.Join(jr.classes, fn)
			os.MkdirAll(filepath.Dir(fn), 0755)
			if err = ioutil.WriteFile(fn, b, 0644); err != nil {
				return errors.Wrap(err, "write "+fn)
			}
		}
		if jr.oracleHome == "" {
			cmd := exec.Command("find", "/oracle", "-type", "f", "-name", "frmjdapi.jar")
			b, _ := cmd.Output()
			if err != nil && len(b) == 0 {
				return errors.Wrapf(err, "%v", cmd.Args)
			}
			jr.oracleHome = filepath.Dir(filepath.Dir(string(bytes.SplitN(b, []byte("\n"), 2)[0])))
		}
		jr.classpath = jr.classes + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmjdapi.jar") + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmxmltools.jar") + ":" +
			filepath.Join(jr.oracleHome, "lib", "xmlparserv2.jar")
	}
	log.Println("classpath:", jr.classpath)

	ctx, cancel := context.WithCancel(jr.globalCtx)
	cmd := exec.CommandContext(ctx, "java", "-cp", jr.classpath,
		"-Djava.library.path="+filepath.Join(jr.oracleHome, "lib"),
		"-Dforms.lib.path="+jr.FormsLibPath,
		"-Dforms.db.conn="+jr.DbConn,
		"unosoft.forms.Serve", "-")
	env := os.Environ()
	for i := 0; i < len(env); i++ {
		e := env[i]
		if i := strings.IndexByte(e, '='); i < 0 {
			env[i] = env[0]
			env = env[1:]
			i--
		} else {
			switch e[:i] {
			case "TERM", "DISPLAY", "PATH", "FORMS_PATH", "LD_LIBRARY_PATH":
				env[i] = env[0]
				env = env[1:]
				i--
			}
		}
	}
	cmd.Env = append(os.Environ(),
		"DISPLAY="+jr.Display,
		"TERM=xterm",
		"FORMS_PATH="+jr.FormsLibPath,
		"PATH="+filepath.Join(jr.oracleHome, "bin")+":"+os.Getenv("PATH"),
		"LD_LIBRARY_PATH="+filepath.Join(jr.oracleHome, "bin")+":"+filepath.Join(jr.oracleHome, "lib")+":"+os.Getenv("LD_LIBRARY_PATH"),
	)
	log.Println(cmd.Env[len(cmd.Env)-4:])
	log.Println(cmd.Args)
	cmd.Stderr = os.Stderr
	jr.cancel = cancel
	{
		pr, pw := io.Pipe()
		cmd.Stdin, jr.request = pr, pw
		go func() {
			<-ctx.Done()
			pw.CloseWithError(ctx.Err())
		}()
	}
	{
		pr, pw := io.Pipe()
		jr.answer, cmd.Stdout = bufio.NewScanner(pr), pw
		go func() {
			<-ctx.Done()
			pw.CloseWithError(ctx.Err())
		}()
	}
	return cmd.Start()
}

func (jr *javaRunner) ConvertFiles(ctx context.Context, dst, src string) error {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	if jr.request == nil {
		if err := jr.start(); err != nil {
			return errors.WithMessage(err, "start")
		}
	}
	if _, err := fmt.Fprintf(jr.request, "%s %s\n", src, dst); err != nil {
		jr.stop()
		return errors.Wrap(err, "write request")
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		for jr.answer.Scan() {
			line := jr.answer.Bytes()
			if bytes.HasPrefix(line, []byte("ERR ")) {
				return errors.New(string(line[4:]))
			}
			if bytes.HasPrefix(line, []byte("OK+ ")) {
				log.Println(string(line[4:]))
				return nil
			}
		}
		return errors.New("no answer")
	})
	return grp.Wait()
}
func (jr *javaRunner) Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error {
	ext, want := "fmb", "xml"
	if mimeType == "application/xml" || mimeType == "text/xml" {
		ext, want = want, ext
	}
	fh, err := ioutil.TempFile("", "forms2xml-*."+ext)
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}
	defer os.Remove(fh.Name())
	want = fh.Name() + "." + want
	if _, err = io.Copy(fh, r); err != nil {
		return errors.Wrap(err, "copy to "+fh.Name())
	}
	if err = fh.Close(); err != nil {
		return errors.Wrap(err, "close "+fh.Name())
	}
	if err = jr.ConvertFiles(ctx, want, fh.Name()); err != nil {
		return errors.WithMessage(err, "convertFiles")
	}
	if fh, err = os.Open(want); err != nil {
		return errors.Wrap(err, "open "+want)
	}
	_, err = io.Copy(w, fh)
	return errors.Wrap(err, "copy from "+fh.Name())
}

type httpClient struct {
	Client *retryablehttp.Client
	URLs   []string
}

func newHTTPClient(urls []string) httpClient {
	cl := httpclient.New("jdapi")
	cl.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, nth int) {
		if nth > 0 {
			logger.Printf("REQUEST[%d] to %q with %q", nth, req.URL, req.Header)
		}
	}
	return httpClient{Client: cl, URLs: urls}
}

func (cl httpClient) Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error {
	URL := cl.URLs[0]
	if len(cl.URLs) > 1 && mimeType == "application/xml" {
		URL = cl.URLs[1]
	}

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read all")
	}
	req, err := retryablehttp.NewRequest("POST", URL, bytes.NewReader(b))
	if err != nil {
		return errors.Wrap(err, URL)
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(b)))
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("Accept", "*/*")
	resp, err := cl.Client.Do(req.WithContext(ctx))
	if err != nil {
		return errors.Wrapf(err, "POST to %q with %q", URL, mimeType)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := iohlp.ReadAll(resp.Body, 1<<20)
		return errors.Wrap(errors.New(resp.Status), string(b))
	}
	if _, err = io.Copy(w, resp.Body); err != nil {
		return errors.Wrap(err, "copy response")
	}
	return nil
}

func (cl httpClient) ConvertFiles(ctx context.Context, dst, src string) error {
	mimeType := "application/xml"
	if strings.HasSuffix(src, ".fmb") {
		mimeType = "application/x-oracle-forms"
	}
	inp, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "open "+src)
	}
	defer inp.Close()
	out, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "create "+dst)
	}
	defer out.Close()
	if err = cl.Convert(ctx, out, inp, mimeType); err != nil {
		return errors.WithMessage(err, "convert")
	}
	return out.Close()
}

type Converter interface {
	Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error
	ConvertFiles(ctx context.Context, dst, src string) error
}

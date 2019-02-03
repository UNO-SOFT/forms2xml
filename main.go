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
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
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

	var concurrency = 4
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

	cmdServe := app.Command("serve", "serve (start java only)")
	cmdServeAddress := cmdServe.Arg("address", "address to listen on").String()

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

	jr := newJavaRunner(ctx, jdapiURLs[0], formsLibPath, display, 0, concurrency)
	converter := Converter(jr)
	log.Println("converter:", converter)

	switch cmd {
	case cmdXML.FullCommand():
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := convertFiles(ctx, converter, *xmlDst, *xmlSrc)
		cancel()
		return err

	case cmdServe.FullCommand():
		http.Handle("/", jr)
		log.Println("Listening on " + *cmdServeAddress)
		return http.ListenAndServe(*cmdServeAddress, nil)

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
	}

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
	MaxRetries                     int
	mu                             sync.Mutex
	classes, classpath, oracleHome string

	newClients  chan HTTPClient
	freeClients chan HTTPClient
}

type HTTPClient struct {
	*retryablehttp.Client
	Cancel context.CancelFunc
	URL    string
	ErrBuf *strings.Builder
}

func (cl HTTPClient) Close() error {
	if cl.Cancel != nil {
		cl.Cancel()
	}
	return nil
}

func newJavaRunner(ctx context.Context, conn, formsLibPath, display string,
	maxRetries, concurrency int) *javaRunner {
	if concurrency <= 1 {
		concurrency = 2
	}
	if maxRetries == 0 {
		maxRetries = 3
	}
	jr := javaRunner{
		DbConn: conn, FormsLibPath: formsLibPath, Display: display,
		newClients:  make(chan HTTPClient, concurrency/2),
		freeClients: make(chan HTTPClient, concurrency/2),
		MaxRetries:  maxRetries,
	}
	go func() {
		for {
			cl, err := jr.start(ctx)
			if err != nil {
				log.Printf("start: %v", err)
				time.Sleep(3 * time.Second)
			}
			select {
			case <-ctx.Done():
				return
			case jr.newClients <- cl:
			}
		}
	}()
	return &jr
}

func (jr *javaRunner) start(ctx context.Context) (cl HTTPClient, err error) {
	if jr.classpath == "" {
		statikFS, err := fs.New()
		if err != nil {
			return cl, errors.Wrap(err, "open statik fs")
		}
		if jr.classes, err = ioutil.TempDir("", "forms2xml-classes-"); err != nil {
			if err != nil {
				return cl, errors.Wrap(err, "create temp dir for classes")
			}
		}

		for _, fn := range []string{"/unosoft/forms/Serve$ConvertHandler.class", "/unosoft/forms/Serve.class"} {
			b, err := fs.ReadFile(statikFS, fn)
			if err != nil {
				return cl, errors.Wrap(err, "read "+fn)
			}
			fn = filepath.Join(jr.classes, fn)
			os.MkdirAll(filepath.Dir(fn), 0755)
			if err = ioutil.WriteFile(fn, b, 0644); err != nil {
				return cl, errors.Wrap(err, "write "+fn)
			}
		}
		if jr.oracleHome == "" {
			cmd := exec.Command("find", "/oracle", "-type", "f", "-name", "frmjdapi.jar")
			b, _ := cmd.Output()
			if err != nil && len(b) == 0 {
				return cl, errors.Wrapf(err, "%v", cmd.Args)
			}
			jr.oracleHome = filepath.Dir(filepath.Dir(string(bytes.SplitN(b, []byte("\n"), 2)[0])))
		}
		jr.classpath = jr.classes + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmjdapi.jar") + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmxmltools.jar") + ":" +
			filepath.Join(jr.oracleHome, "lib", "xmlparserv2.jar")
	}
	log.Println("classpath:", jr.classpath)

	netAddr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return cl, err
	}
	l, err := net.ListenTCP("tcp", netAddr)
	if err != nil {
		return cl, err
	}
	addr := l.Addr().(*net.TCPAddr).String()
	l.Close()

	env := os.Environ()
	for i := 0; i < len(env); i++ {
		e := env[i]
		if i := strings.IndexByte(e, '='); i < 0 {
			env[i] = env[0]
			env = env[1:]
			i--
		} else {
			switch e[:i] {
			case "TERM", "DISPLAY", "PATH", "FORMS_PATH", "ORACLE_HOME", "LD_LIBRARY_PATH":
				env[i] = env[0]
				env = env[1:]
				i--
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, "java", "-cp", jr.classpath,
		"-Djava.library.path="+filepath.Join(jr.oracleHome, "lib"),
		"-Dforms.lib.path="+jr.FormsLibPath,
		"-Dforms.db.conn="+jr.DbConn,
		"unosoft.forms.Serve", addr)
	cmd.Env = append(os.Environ(),
		"DISPLAY="+jr.Display,
		"TERM=xterm",
		"FORMS_PATH="+jr.FormsLibPath,
		"PATH="+filepath.Join(jr.oracleHome, "bin")+":/usr/lib64/qt-3.3/bin:"+os.Getenv("PATH"),
		"ORACLE_HOME="+jr.oracleHome,
		"LD_LIBRARY_PATH="+filepath.Join(jr.oracleHome, "bin")+":"+filepath.Join(jr.oracleHome, "lib")+":"+os.Getenv("LD_LIBRARY_PATH"),
	)
	log.Println(cmd.Env[len(cmd.Env)-5:])
	log.Println(cmd.Args)
	cl.ErrBuf = &strings.Builder{}
	cmd.Stderr = cl.ErrBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}

	cl.Cancel = func() {
		log.Println("CANCEL " + addr)
		cancel()
	}
	cl.Client = retryablehttp.NewClient()
	cl.Client.RetryMax = 1
	cl.Client.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, nth int) {
		if nth > 0 {
			logger.Printf("REQUEST[%d] to %q with %q", nth, req.URL, req.Header)
		}
	}
	cl.URL = "http://" + addr

	return cl, cmd.Start()
}

func (jr *javaRunner) NewClient(ctx context.Context) HTTPClient {
	var cl HTTPClient
	select {
	case cl = <-jr.freeClients:
	default:
	}
	if cl.Client == nil {
		cl = <-jr.newClients
	}
	go func() { <-ctx.Done(); cl.Cancel() }()
	return cl
}

func (jr *javaRunner) Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error {
	b, err := iohlp.ReadAll(r, 1<<20)
	if err != nil {
		return errors.Wrap(err, "read all")
	}
	resp, err := jr.do(ctx, func(URL string) (*retryablehttp.Request, error) {
		req, err := retryablehttp.NewRequest("POST", URL, b)
		if err != nil {
			return nil, errors.Wrap(err, URL)
		}
		req.Header.Set("Content-Length", strconv.Itoa(len(b)))
		req.Header.Set("Content-Type", mimeType)
		req.Header.Set("Accept", "*/*")
		return req, nil
	})
	if err != nil {
		return err
	}

	var status, URL string
	if resp != nil {
		status = resp.Status
		if resp.Request != nil {
			URL = resp.Request.URL.String()
		}
		defer resp.Body.Close()
	}
	log.Printf("POST[%s] %d bytes to %q: %s: %v", mimeType, len(b), URL, status, err)
	if err != nil {
		return errors.Wrapf(err, "POST to %q with %q", URL, mimeType)
	}
	if resp.StatusCode >= 400 {
		b, _ := iohlp.ReadAll(resp.Body, 1<<20)
		return errors.Wrap(errors.New(resp.Status), string(b))
	}
	fn := strings.TrimPrefix(resp.Header.Get("Location"), "file://")
	if fn == "" {
		_, err = io.Copy(w, resp.Body)
		return errors.Wrap(err, "copying from response")
	}
	fh, err := os.Open(fn)
	if err != nil {
		return errors.Wrap(err, resp.Header.Get("Location"))
	}
	defer fh.Close()
	os.Remove(fh.Name())
	log.Printf("copying from %q...", fh.Name())
	n, err := io.Copy(w, fh)
	log.Printf("copied %d bytes: %v", n, err)
	return errors.Wrap(err, "copy response")
}

func (jr *javaRunner) do(ctx context.Context, makeRequest func(address string) (*retryablehttp.Request, error)) (*http.Response, error) {
	var err error
	var resp *http.Response
	for i := 0; i < jr.MaxRetries; i++ {
		cl := jr.NewClient(context.Background())
		var req *retryablehttp.Request
		if req, err = makeRequest(cl.URL); err != nil {
			return nil, errors.WithMessage(err, cl.URL)
		}
		if resp, err = cl.Do(req); err == nil {
			defer func() {
				select {
				case jr.freeClients <- cl:
				default:
					cl.Close()
				}
			}()
			return resp, nil
		}
		cl.Close()
		err = errors.Wrap(err, cl.ErrBuf.String())
		log.Println(err)
		time.Sleep(1 * time.Second)
	}
	return nil, err
}

func (jr *javaRunner) ConvertFiles(ctx context.Context, dst, src string) error {
	resp, err := jr.do(ctx, func(URL string) (*retryablehttp.Request, error) {
		URL += "?src=" + url.QueryEscape(src) + "&dst=" + url.QueryEscape(dst)
		return retryablehttp.NewRequest("GET", URL, nil)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := iohlp.ReadAll(resp.Body, 1<<20)
		return errors.Wrap(errors.New(resp.Status), string(b))
	}
	return nil
}

func (jr *javaRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := iohlp.ReadAll(r.Body, 1<<20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	uri := r.URL.RequestURI()
	resp, err := jr.do(r.Context(), func(URL string) (*retryablehttp.Request, error) {
		URL += uri
		return retryablehttp.NewRequest(r.Method, URL, b)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		w.Header()[k] = vv
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

type Converter interface {
	Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error
	ConvertFiles(ctx context.Context, dst, src string) error
}

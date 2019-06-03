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

//go:generate env OHOME=/oracle/fmw12c/product sh -c "set -x; javac -cp classes:${DOLLAR}OHOME/jlib/frmjdapi.jar:${DOLLAR}OHOME/jlib/frmxmltools.jar:${DOLLAR}OHOME/oracle_common/modules/oracle.xdk/xmlparserv2.jar -d classes src/unosoft/forms/Serve.java"

//go:generate statik  -m -f -p statik -src classes

package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/rjeczalik/notify"
	"golang.org/x/sync/errgroup"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/UNO-SOFT/forms2xml/transform"
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
	cmdServeAddress := cmdServe.Arg("address", "address to listen on").Required().String()

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
	watchServeAddress := cmdWatch.Flag("http", "HTTP address to listen on").String()

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
	jr.MaxRetries = 2
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
		http.Handle("/", jr)
		grp, ctx := errgroup.WithContext(ctx)
		if *watchServeAddress != "" {
			grp.Go(func() error {
				log.Println("Listening on " + *watchServeAddress)
				return http.ListenAndServe(*watchServeAddress, nil)
			})
		}
		grp.Go(func() error {
			return watchConvert(ctx, converter, watchDst, watchSrc, !*watchNoTransform, fileSuffix, concurrency)
		})
		return grp.Wait()
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

type Converter interface {
	Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error
	ConvertFiles(ctx context.Context, dst, src string) error
}

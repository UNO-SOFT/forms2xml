// Copyright 2018 Tamás Gulácsi
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

package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	"github.com/rjeczalik/notify"
	"golang.org/x/sync/errgroup"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/UNO-SOFT/forms2xml/transform"
)

func main() {
	if err := Main(); err != nil {
		log.Fatal(err)
	}
}

var jdapiURLs = []string{"http://localhost:8008"}

func Main() error {
	if len(jdapiURLs) == 1 {
		jdapiURLs = append(jdapiURLs, jdapiURLs[0])
	}

	var concurrency = 8
	app := kingpin.New("forms2xml", "Oracle Forms .fmb <-> .xml with optional conversion")
	app.Flag("jdapi-src", "SRC Form JDAPI helper HTTP listener URL").
		Default(jdapiURLs[0]).StringVar(&jdapiURLs[0])
	app.Flag("jdapi-dst", "DEST Form JDAPI helper HTTP listener URL").
		Default(jdapiURLs[1]).StringVar(&jdapiURLs[1])

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
	switch cmd {
	case cmdXML.FullCommand():
		return convertFiles(*xmlDst, *xmlSrc)

	case cmdTransform.FullCommand():
		return transformFiles(*tranDst, *tranSrc)

	case cmd6211.FullCommand():
		return convertFiles6to11(*upDst, *upSrc, !*upNoTransform, *upSuffix)

	case cmdWatch.FullCommand():
		return watchConvert(watchDst, watchSrc, !*watchNoTransform, fileSuffix, concurrency)
	}
	return nil
}

func watchConvert(dstDir, srcDir string, doTransform bool, suffix string, concurrency int) error {
	tokens := make(chan struct{}, concurrency)
	eventCh := make(chan notify.EventInfo, 16)
	if err := notify.Watch(srcDir, eventCh, eventsToWatch...); err != nil {
		return err
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
			return err
		}
	}
	defer inp.Close()

	out := os.Stdout
	if !(dst == "" || dst == "-") {
		var err error
		if out, err = os.Create(dst); err != nil {
			return err
		}
	}
	defer out.Close()

	var P transform.FormsXMLProcessor
	if err := P.ProcessStream(out, inp); err != nil {
		return err
	}
	return out.Close()
}

func convertFiles6to11(dst, src string, doTransform bool, suffix string) error {
	if dst == "" {
		dst = strings.TrimSuffix(src, ".fmb") + suffix + ".fmb"
	}
	if dst == src {
		return errors.Wrap(errors.New("overwrite source file"), src)
	}
	log.Printf("Convert %q to %q.", src, dst)
	inp, err := os.Open(src)
	if err != nil {
		return err
	}
	defer inp.Close()
	if dfi, err := os.Stat(dst); err == nil {
		sfi, err := inp.Stat()
		if err != nil {
			return err
		}
		if os.SameFile(sfi, dfi) {
			return errors.Wrap(errors.New("overwrite source file"), sfi.Name())
		}
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
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
				return err
			})
		}
		log.Println("start convert")
		err := convert(out, xmlSource, "application/xml")
		log.Printf("xml->fmb: %+v", err)
		xr.CloseWithError(err)
		return err
	})
	err = convert(xw, inp, "application/x-oracle-forms")
	log.Printf("fmb->xml: %+v", err)
	xw.CloseWithError(err)
	if err != nil {
		return err
	}
	if err = grp.Wait(); err != nil {
		return err
	}
	return out.Close()
}

func convertFiles(dst, src string) error {
	mimeType := "application/x-oracle-forms"
	inp := io.ReadCloser(os.Stdin)
	var err error
	if src != "" && src != "-" {
		if inp, err = os.Open(src); err != nil {
			return err
		}
		if strings.HasSuffix(src, ".xml") {
			mimeType = "application/xml"
		}
	} else {
		var a [1024]byte
		n, err := io.ReadAtLeast(inp, a[:], 4)
		if err != nil {
			return err
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
			return err
		}
		defer out.Close()
	}

	if err = convert(out, inp, mimeType); err != nil {
		return err
	}
	return out.Close()
}

var httpClient = retryablehttp.NewClient()

func convert(w io.Writer, r io.Reader, mimeType string) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	httpClient.RequestLogHook = func(logger *log.Logger, req *http.Request, nth int) {
		if nth > 0 {
			logger.Printf("REQUEST[%d] to %q with %q", nth, req.URL, req.Header)
		}
	}
	URL := jdapiURLs[0]
	if len(jdapiURLs) > 1 && mimeType == "application/xml" {
		URL = jdapiURLs[1]
	}
	req, err := retryablehttp.NewRequest("POST", URL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(b)))
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("Accept", "*/*")
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "POST to %q with %q", URL, mimeType)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := ioutil.ReadAll(resp.Body)
		return errors.Wrap(errors.New(resp.Status), string(b))
	}
	if _, err = io.Copy(w, resp.Body); err != nil {
		return err
	}
	return nil
}

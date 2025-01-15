// Copyright 2019, 2025 Tamás Gulácsi
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

package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/rjeczalik/notify"
	"golang.org/x/sync/errgroup"

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

	srcDst := func(args []string) (string, string, error) {
		if len(args) == 0 {
			return "", "", fmt.Errorf("source file is required")
		}
		src := args[0]
		var dst string
		if len(args) > 0 {
			dst = args[1]
		}
		return src, dst, nil
	}

	var converter Converter
	cmdXML := ff.Command{Name: "xml", ShortHelp: "convert to-from XML",
		Usage: "xml <source file> [destination file]",
		Exec: func(ctx context.Context, args []string) error {
			xmlSrc, xmlDst, err := srcDst(args)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = convertFiles(ctx, converter, xmlDst, xmlSrc)
			cancel()
			return err
		},
	}

	var jr *javaRunner
	cmdServe := ff.Command{Name: "serve", ShortHelp: "serve (start java only)",
		Usage: "serve <address to listen on>",
		Exec: func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("address is required")
			}
			cmdServeAddress := args[0]
			http.Handle("/", jr)
			log.Println("Listening on " + cmdServeAddress)
			server := http.Server{Addr: cmdServeAddress}
			go func() {
				<-ctx.Done()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				server.Shutdown(ctx)
				cancel()
			}()
			return server.ListenAndServe()
		},
	}

	cmdTransform := ff.Command{Name: "transform", ShortHelp: "transform the XML",
		Usage: "transform <source file> [destination file]",
		Exec: func(ctx context.Context, args []string) error {
			tranSrc, tranDst, err := srcDst(args)
			if err != nil {
				return err
			}
			return transformFiles(tranDst, tranSrc)
		},
	}

	FS := ff.NewFlagSet("6to11")
	upNoTransform := FS.Bool('n', "no-transform", "don't transform")
	upSuffix := FS.String('S', "suffix", "-v11", "suffix of converted files")
	cmd6211 := ff.Command{Name: "6to11", Flags: FS,
		ShortHelp: "convert from Forms v6 to v11",
		Exec: func(ctx context.Context, args []string) error {
			upSrc, upDst, err := srcDst(args)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err = convertFiles6to11(ctx, converter, upDst, upSrc, !*upNoTransform, *upSuffix)
			cancel()
			return err
		},
	}

	var watchSrc, watchDst string
	FS = ff.NewFlagSet("watch")
	watchFileSuffix := FS.String('S', "suffix", "-v11", "suffix of converted files")
	watchNoTransform := FS.BoolDefault('n', "no-transform", false, "don't transform")
	FS.IntVar(&concurrency, 0, "concurrency", concurrency, "maximum number of conversions running in parallel")
	watchServeAddress := FS.String(0, "http", "", "HTTP address to listen on")
	cmdWatch := ff.Command{Name: "watch", Flags: FS,
		ShortHelp: "watch a directory and transform all appearing files",
		Exec: func(ctx context.Context, args []string) error {
			var err error
			watchSrc, watchDst, err = srcDst(args)
			if err != nil {
				return err
			}
			http.Handle("/", jr)
			grp, ctx := errgroup.WithContext(ctx)
			if *watchServeAddress != "" {
				grp.Go(func() error {
					log.Println("Listening on " + *watchServeAddress)
					return http.ListenAndServe(*watchServeAddress, nil)
				})
			}
			grp.Go(func() error {
				return watchConvert(ctx, converter, watchDst, watchSrc, !*watchNoTransform, *watchFileSuffix, concurrency)
			})
			return grp.Wait()
		},
	}

	FS = ff.NewFlagSet("forms2xml")
	FS.StringVar(&jdapiURLs[0], 0, "jdapi-src", jdapiURLs[0], "SRC Form JDAPI helper HTTP listener URL")
	FS.StringVar(&jdapiURLs[1], 0, "jdapi-dst", jdapiURLs[1], "DEST Form JDAPI helper HTTP listener URL")
	FS.StringVar(&formsLibPath, 0, "forms.lib.path", formsLibPath, "FORMS_PATH")
	FS.StringVar(&display, 0, "display", os.Getenv("DISPLAY"), "DISPLAY")
	app := ff.Command{Name: "forms2xml", Flags: FS,
		ShortHelp:   "Oracle Forms .fmb <-> .xml with optional conversion",
		Exec:        cmdXML.Exec,
		Subcommands: []*ff.Command{&cmdXML, &cmdServe, &cmdTransform, &cmd6211, &cmdWatch},
	}

	if err := app.Parse(os.Args[1:]); err != nil {
		ffhelp.Command(&app).WriteTo(os.Stderr)
		if errors.Is(err, ff.ErrHelp) {
			return nil
		}
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	jr = newJavaRunner(ctx, jdapiURLs[0], formsLibPath, display, 0, concurrency)
	jr.MaxRetries = 2
	converter = Converter(jr)
	log.Println("converter:", converter)

	return app.Run(ctx)
}

func watchConvert(ctx context.Context, converter Converter, dstDir, srcDir string, doTransform bool, suffix string, concurrency int) error {
	tokens := make(chan struct{}, concurrency)
	eventCh := make(chan notify.EventInfo, 16)
	if err := notify.Watch(srcDir, eventCh, eventsToWatch...); err != nil {
		return fmt.Errorf("watch: %w", err)
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
			return fmt.Errorf("open %q: %w", src, err)
		}
	}
	defer inp.Close()

	out := os.Stdout
	if !(dst == "" || dst == "-") {
		var err error
		if out, err = os.Create(dst); err != nil {
			return fmt.Errorf("create %q: %w", dst, err)
		}
	}
	defer out.Close()

	var P transform.FormsXMLProcessor
	if err := P.ProcessStream(out, inp); err != nil {
		return fmt.Errorf("processStream: %w", err)
	}
	return out.Close()
}

func convertFiles6to11(ctx context.Context, converter Converter, dst, src string, doTransform bool, suffix string) error {
	if dst == "" {
		dst = strings.TrimSuffix(src, ".fmb") + suffix + ".fmb"
	}
	if dst == src {
		return fmt.Errorf("overwrite source file %q", src)
	}
	log.Printf("Convert %q to %q.", src, dst)
	inp, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer inp.Close()
	if dfi, err := os.Stat(dst); err == nil {
		sfi, err := inp.Stat()
		if err != nil {
			return fmt.Errorf("stat %q: %w", dst, err)
		}
		if os.SameFile(sfi, dfi) {
			return fmt.Errorf("overwrite source file %q", sfi.Name())
		}
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
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
				if err != nil {
					return fmt.Errorf("processStream: %w", err)
				}
				return err
			})
		}
		log.Println("start convert")
		err := converter.Convert(ctx, out, xmlSource, "application/xml")
		log.Printf("xml->fmb: %+v", err)
		xr.CloseWithError(err)
		if err != nil {
			return fmt.Errorf("convert: %w", err)
		}
		return nil
	})
	err = converter.Convert(ctx, xw, inp, "application/x-oracle-forms")
	log.Printf("fmb->xml: %+v", err)
	xw.CloseWithError(err)
	if err != nil {
		return fmt.Errorf("convert: %w", err)
	}
	if err = grp.Wait(); err != nil {
		return fmt.Errorf("convertFiles6to11: %w", err)
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
		return fmt.Errorf("readAtLeast stdin: %w", err)
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
			return fmt.Errorf("create %q: %w", dst, err)
		}
		defer out.Close()
	}

	if err = converter.Convert(ctx, out, inp, mimeType); err != nil {
		return fmt.Errorf("convertFiles: %w", err)
	}
	return out.Close()
}

type Converter interface {
	Convert(ctx context.Context, w io.Writer, r io.Reader, mimeType string) error
	ConvertFiles(ctx context.Context, dst, src string) error
}

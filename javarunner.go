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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"

	"github.com/tgulacsi/go/iohlp"

	_ "github.com/UNO-SOFT/forms2xml/statik"
	"github.com/rakyll/statik/fs"
)

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
	var mimeType string
	if r.Method == "POST" {
		mimeType = r.Header.Get("Content-Type")
		if mimeType == "" || !(mimeType == "application/xml" || mimeType == "application/x-oracle-forms") {
			mimeType = "application/x-oracle-forms"
			if bytes.HasPrefix(bytes.TrimSpace(b[:1024]), []byte("<?xml")) {
				mimeType = "application/xml"
			}
		}
	}
	uri := r.URL.RequestURI()
	resp, err := jr.do(r.Context(), func(URL string) (*retryablehttp.Request, error) {
		URL += uri
		req, err := retryablehttp.NewRequest(r.Method, URL, b)
		if err != nil {
			return nil, err
		}
		if mimeType != "" {
			req.Header.Set("Content-Type", mimeType)
		}
		log.Println(r.Method, URL, mimeType, len(b))
		return req, nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		w.Header()[k] = vv
	}
	log.Println(resp.Status, resp.Header)
	if resp.StatusCode == 201 {
		if loc := strings.TrimPrefix(resp.Header.Get("Location"), "file://"); loc != "" {
			if fh, err := os.Open(loc); err == nil {
				if strings.HasPrefix(fh.Name(), os.TempDir()) {
					os.Remove(fh.Name())
				}
				w.WriteHeader(resp.StatusCode)
				io.Copy(w, fh)
				fh.Close()
				return
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

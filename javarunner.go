package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

/*
export CT_JAVA_HOME="/oracle/fmw12c/product/jdk"
export DISPLAY="aix-dev-ab7.unosoft.local:0"
export DOMAIN_HOME="/oracle/fmw12c/config/domains/bruno"
export FORMS_API_TK_BYPASS="false"
export FORMS_INSTANCE="/oracle/fmw12c/config/domains/bruno/config/fmwconfig/components/FORMS/instances/forms1"
export FORMS_PATH="/home/aegon/dev/lib"
export HISTSIZE="1000"
export HOME="/home/oracle"
export HOSTNAME="lnx-dev-ae7.unosoft.local"
export LANG="en_US.iso88592"
export LD_LIBRARY_PATH="/oracle/fmw12c/product/jdk/jre/lib/amd64/native_threads:/oracle/fmw12c/product/jdk/jre/lib/amd64/server:/oracle/fmw12c/product/jdk/jre/lib/amd64:/oracle/fmw12c/product/wlserver/../lib"
export LOGNAME="oracle"
export LS_COLORS="rs=0:di=01;34:ln=01;36:mh=00:pi=40;33:so=01;35:do=01;35:bd=40;33;01:cd=40;33;01:or=40;31;01:mi=01;05;37;41:su=37;41:sg=30;43:ca=30;41:tw=30;42:ow=34;42:st=37;44:ex=01;32:*.tar=01;31:*.tgz=01;31:*.arc=01;31:*.arj=01;31:*.taz=01;31:*.lha=01;31:*.lz4=01;31:*.lzh=01;31:*.lzma=01;31:*.tlz=01;31:*.txz=01;31:*.tzo=01;31:*.t7z=01;31:*.zip=01;31:*.z=01;31:*.Z=01;31:*.dz=01;31:*.gz=01;31:*.lrz=01;31:*.lz=01;31:*.lzo=01;31:*.xz=01;31:*.bz2=01;31:*.bz=01;31:*.tbz=01;31:*.tbz2=01;31:*.tz=01;31:*.deb=01;31:*.rpm=01;31:*.jar=01;31:*.war=01;31:*.ear=01;31:*.sar=01;31:*.rar=01;31:*.alz=01;31:*.ace=01;31:*.zoo=01;31:*.cpio=01;31:*.7z=01;31:*.rz=01;31:*.cab=01;31:*.jpg=01;35:*.jpeg=01;35:*.gif=01;35:*.bmp=01;35:*.pbm=01;35:*.pgm=01;35:*.ppm=01;35:*.tga=01;35:*.xbm=01;35:*.xpm=01;35:*.tif=01;35:*.tiff=01;35:*.png=01;35:*.svg=01;35:*.svgz=01;35:*.mng=01;35:*.pcx=01;35:*.mov=01;35:*.mpg=01;35:*.mpeg=01;35:*.m2v=01;35:*.mkv=01;35:*.webm=01;35:*.ogm=01;35:*.mp4=01;35:*.m4v=01;35:*.mp4v=01;35:*.vob=01;35:*.qt=01;35:*.nuv=01;35:*.wmv=01;35:*.asf=01;35:*.rm=01;35:*.rmvb=01;35:*.flc=01;35:*.avi=01;35:*.fli=01;35:*.flv=01;35:*.gl=01;35:*.dl=01;35:*.xcf=01;35:*.xwd=01;35:*.yuv=01;35:*.cgm=01;35:*.emf=01;35:*.axv=01;35:*.anx=01;35:*.ogv=01;35:*.ogx=01;35:*.aac=01;36:*.au=01;36:*.flac=01;36:*.mid=01;36:*.midi=01;36:*.mka=01;36:*.mp3=01;36:*.mpc=01;36:*.ogg=01;36:*.ra=01;36:*.wav=01;36:*.axa=01;36:*.oga=01;36:*.spx=01;36:*.xspf=01;36:"
export MAIL="/var/spool/mail/tgulacsi"
export OLDPWD
export ORACLE_HOME="/oracle/fmw12c/product/wlserver/.."
export O_JDK_HOME="/oracle/fmw12c/product/jdk"
export PATH="/sbin:/bin:/usr/sbin:/usr/bin"
export PWD="/home/aegon/dev"
export SHELL="/usr/bin/bash"
export SHLVL="1"
export SUDO_COMMAND="/bin/env FORMS_PATH=/home/aegon/dev/lib DISPLAY=aix-dev-ab7.unosoft.local:0 FORMS_API_TK_BYPASS=false /oracle/fmw12c/config/domains/bruno/./config/fmwconfig/components/FORMS/instances/forms1/bin/frmxml2f.sh userid=bruno_owner/xxx@ae7_dev overwrite=yes /tmp/x.xml /tmp/x.fmb"
export SUDO_GID="10508"
export SUDO_UID="11056"
export SUDO_USER="tgulacsi"
export TERM="xterm-256color"
export TNS_ADMIN="/oracle/fmw12c/config/domains/bruno/config/fmwconfig"
export USER="oracle"
export USERNAME="oracle"
export XDG_SESSION_ID="3798"
+ /oracle/fmw12c/product/jdk/bin/java -classpath /oracle/fmw12c/product/wlserver/../jlib/frmjdapi.jar:/oracle/fmw12c/product/wlserver/../jlib/frmxmltools.jar:/oracle/fmw12c/product/wlserver/../oracle_common/modules/oracle.xdk/xmlparserv2.jar oracle.forms.util.xmltools.XML2Forms userid=bruno_owner/xxx@ae7_dev overwrite=yes /tmp/x.xml /tmp/x.fmb
*/

func (jr *javaRunner) start(ctx context.Context) (cl HTTPClient, err error) {
	if jr.classpath == "" {
		statikFS, err := fs.New()
		if err != nil {
			return cl, errors.Wrap(err, "open statik fs")
		}
		if jr.classes, err = os.MkdirTemp("", "forms2xml-classes-"); err != nil {
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
			if err = os.WriteFile(fn, b, 0644); err != nil {
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
		// /oracle/fmw12c/product/jdk/bin/java -classpath /oracle/fmw12c/product/wlserver/../jlib/frmjdapi.jar:/oracle/fmw12c/product/wlserver/../jlib/frmxmltools.jar:/oracle/fmw12c/product/wlserver/../oracle_common/modules/oracle.xdk/xmlparserv2.jar oracle.forms.util.xmltools.XML2Forms userid=bruno_owner/xxx@ae7_dev overwrite=yes /tmp/x.xml /tmp/x.fmb

		jr.classpath = jr.classes + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmjdapi.jar") + ":" +
			filepath.Join(jr.oracleHome, "jlib", "frmxmltools.jar")
		for _, dn := range []string{"oracle_common/modules/oracle.xdk", "lib"} {
			fn := filepath.Join(jr.oracleHome, dn, "xmlparserv2.jar")
			if _, err := os.Stat(fn); err == nil {
				jr.classpath += ":" + fn
				break
			}
		}
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
	b, closer, err := iohlp.ReadAll(r, 1<<20)
	if err != nil {
		return errors.Wrap(err, "read all")
	}
	defer closer.Close()
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
		b, closer, _ := iohlp.ReadAll(resp.Body, 1<<20)
		s := string(b)
		closer.Close()
		return errors.Wrap(errors.New(resp.Status), s)
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
	if resp.StatusCode >= 400 {
		b, closer, _ := iohlp.ReadAll(resp.Body, 1<<20)
		s := string(b)
		closer.Close()
		return errors.Wrap(errors.New(resp.Status), s)
	}
	return nil
}

func (jr *javaRunner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, closer, err := iohlp.ReadAll(r.Body, 1<<20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer closer.Close()
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

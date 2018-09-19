package transform_test

import (
	"encoding/xml"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/UNO-SOFT/forms2xml/transform"
	"github.com/google/go-cmp/cmp"
)

func TestProcess(t *testing.T) {
	fh, err := os.Open(flag.Arg(0))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var P transform.FormsXMLProcessor
	var buf strings.Builder
	var in strings.Builder
	if err := P.Process(xml.NewEncoder(&buf), xml.NewDecoder(io.TeeReader(fh, &in))); err != nil {
		t.Fatal(err)
	}
	outS := buf.String()
	//t.Log(outS)

	if i := strings.Index(outS, "C_STCK_CONTENT"); i >= 0 {
		i -= 100
		if i < 0 {
			i = 0
		}
		t.Errorf("C_STCK_CONTENT remained: " + outS[i:i+200])
	}

	inTokens, err := startElements(strings.NewReader(in.String()))
	if err != nil {
		t.Fatal(err)
	}
	//t.Log("in:", inTokens)
	outTokens, err := startElements(strings.NewReader(outS))
	if err != nil {
		t.Fatal(err)
	}
	//t.Log("out:", outTokens)

	if diff := cmp.Diff(inTokens, outTokens); diff != "" {
		t.Log(diff)
	}
}

func TestParse(t *testing.T) {
	flag.Parse()
	var in strings.Builder
	fh, err := os.Open(flag.Arg(0))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var P transform.FormsXMLProcessor
	var out strings.Builder
	if err = P.ProcessStream(&out, io.TeeReader(fh, &in)); err != nil {
		t.Fatal(err)
	}
	outS := out.String()
	//io.WriteString(os.Stdout, outS)

	inTokens, err := startElements(strings.NewReader(in.String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("in:", inTokens)
	outTokens, err := startElements(strings.NewReader(outS))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("out:", outTokens)

	if diff := cmp.Diff(inTokens, outTokens); diff != "" {
		t.Log(diff)
	}
}

func startElements(r io.Reader) ([]string, error) {
	var ss []string
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ss, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			ss = append(ss, se.Name.Local)
		}
	}
	return ss, nil
}

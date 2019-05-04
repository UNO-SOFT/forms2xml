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

package transform

import (
	"bytes"
	"encoding/xml"
	"io"
	//"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const DefaultCellWidth, DefaultCellHeight = 12, 24

var DefaultUsedVisualAttributes = []string{
	"NORMAL_ITEM", "NORMAL_ITEM12", 
	"SELECT", "NORMAL_PROMPT", "NORMAL",
	"DISPLAY_ITEM12",
	"NORMAL_CANVAS", 
	"NORMAL_TITLE", "NORMAL_TITLE12", 
	"PROMPT_TITLE", "PROMPT_TITLE12", 
	"PROMPT_ITEM", "PROMPT_ITEM12",
}

type FormsXMLProcessor struct {
	CellWidth, CellHeight uint8
	UsedVisualAttributes  map[string]struct{}
	UnknownParents        map[string]struct{}

	//Module Module

	missingVAs    map[string]struct{}
	missingParams map[string]struct{}

	seen  []string
	stack []string

	tbdPromptVAs map[string]struct{}
	tbdVAs       map[string]struct{}
}

func (P *FormsXMLProcessor) ProcessStream(w io.Writer, r io.Reader) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return P.Process(enc, xml.NewDecoder(r))
}

func (P *FormsXMLProcessor) Process(enc *xml.Encoder, dec *xml.Decoder) error {
	if P.CellWidth == 0 {
		P.CellWidth = DefaultCellWidth
	}
	if P.CellHeight == 0 {
		P.CellHeight = DefaultCellHeight
	}
	if P.UsedVisualAttributes == nil {
		P.UsedVisualAttributes = make(map[string]struct{}, len(DefaultUsedVisualAttributes))
		for _, a := range DefaultUsedVisualAttributes {
			P.UsedVisualAttributes[a] = struct{}{}
		}
	}
	if P.missingVAs == nil {
		P.missingVAs = make(map[string]struct{})
	}
	if P.missingParams == nil {
		P.missingParams = make(map[string]struct{})
		for _, p := range RequiredParams {
			P.missingParams[p] = struct{}{}
		}
	}
	/* # TODO:
	Menu Module M_MENU

	*/

	/*
	   	# TODO: !
	       # 1. Form.Physical.Coordinate System nél a systemet pixel-re
	       # 					a width-et 12 -re
	       # 					a height-et 24 -ra álltíani
	       #     Form.Functional.Console Window-t W_main-re állítani
	       # 2. BR_FLIB-ből átmásolni a C_CONTENT-et a Canvases-ba
	       # 3. A mezőknél a Physical.Canvas-t  átírni C_CONTENT-re
	       # 4. BR_FLIB-ből a
	       # NORMAL_ITEM
	       # SELECT
	       # NORMAL_PROMPT
	       # NORMAL
	       # Visual Attributes-ban lévő atribútumokat áthúzni
	       # 5. A BLOCK-oknál a Record.Current Record Visual Attribute Group-ot SELECT-re váltani
	       # 6. A mezőknél a Visual Attributes-ban :
	       # A Visual Attriburte Group : NORMAL_ITEM r állítani
	       # A Prompt Visual Attribute Group :  NORMAL_PROMPT ra állítani
	       # 7. WINDOW-ba inheritálni kell egy csomó attribútumot (miket ?)
	       # 8. Mező szélesség kb. 10x hossz + 6
	       # 9. Ha a Canvas-on a mezők kerete nem látszik a mező property
	       # palette-en :
	       # Physical.Bevel : Lowered re
	       # Physical.Rendered YES -re
	*/

Loop:
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "read")
		}
		switch st := tok.(type) {
		case xml.StartElement:
			st.Name.Space = ""
			P.stack = append(P.stack, st.Name.Local)
			err = P.processStartElement(enc, &st)
			st.Attr = fixAttrs(st.Attr)
			tok = st
			P.seen = append(P.seen, strings.Join(P.stack, "/"))
			if err != nil {
				if errors.Cause(err) == errSkipElement {
					dec.Skip()
					continue Loop
				}
				return err
			}
		case xml.EndElement:
			st.Name.Space = ""
			tok = st
			P.stack = P.stack[:len(P.stack)-1]
		case xml.CharData:
			st = xml.CharData(bytes.TrimSpace(st))
			tok = st
		}
		if err := enc.EncodeToken(tok); err != nil {
			return errors.Wrap(err, "encode")
		}
	}
	return enc.Flush()
}

var errSkipElement = errors.New("skip element")

func (P *FormsXMLProcessor) processStartElement(enc *xml.Encoder, st *xml.StartElement) error {
	if len(P.seen) != 0 && strings.HasSuffix(P.seen[len(P.seen)-1], "/FormModule") {
		P.attachLibs(enc)
	}
	if va := getAttr(st.Attr, "VisualAttribute"); va != "" {
		P.missingVAs[va] = struct{}{}
	}
	switch st.Name.Local {
	case "VisualAttribute":
		P.addTriggers(enc)
		delete(P.missingVAs, getAttr(st.Attr, "Name"))
	case "ModuleParameter":
		delete(P.missingParams, getAttr(st.Attr, "Name"))
	case "Window":
		vas := make(map[string]struct{}, len(P.UsedVisualAttributes)+len(P.missingVAs))
		for s := range P.UsedVisualAttributes {
			vas[s] = struct{}{}
		}
		for s := range P.missingVAs {
			vas[s] = struct{}{}
		}
		if err := P.addMissingVAs(enc, vas); err != nil {
			return err
		}
	}
	if len(P.missingParams) != 0 {
		switch st.Name.Local {
		case "LOV", "ProgramUnit", "PropertyClass", "RecordGroup", "VisualAttribute", "Window":
			if err := P.addMissingParams(enc, P.missingParams); err != nil {
				return err
			}
			for k := range P.missingParams {
				delete(P.missingParams, k)
			}
		}
	}

	if err := P.removeExcessAlert(st); err != nil {
		return err
	}
	P.fixStackedCanvas(st)
	P.fixCoordinate(st)

	P.scaleElt(st)
	P.fixParentModule(st)
	P.subclassRootwindow(st)
	P.fixBadItemType(st)
	P.trimSpaces(st)
	P.fixVAs(st)
	P.fixPromptVAs(st)

	return nil
}

type Trigger struct {
	Name             string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	ParentType       string     `xml:",attr,omitempty"`
	TriggerText      string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
}

func (P *FormsXMLProcessor) addTriggers(enc *xml.Encoder) error {
	for _, nm := range []string{
		"ON-MESSAGE", "ON-ERROR",
		"KEY-SCRUP", "KEY-SCRDOWN", "KEY-PREV-ITEM", "KEY-NEXT-ITEM",
		"KEY-UP", "KEY-DOWN", "KEY-OTHERS",
		"PRE-FORM",
	} {
		if err := enc.Encode(Trigger{Name: nm,
			ParentModule: "BR_FLIB", ParentModuleType: "12",
			ParentName: nm, ParentFilename: "BR_FLIB.fmb", ParentType: "37",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (P *FormsXMLProcessor) removeExcessAlert(st *xml.StartElement) error {
	if st.Name.Local != "Alert" {
		return nil
	}
	switch getAttr(st.Attr, "Name") {
	case "KERDEZ_ALERT", "UZEN_ALERT":
		return errSkipElement
	}
	return nil
}

var stackedCanvasAttrs = map[string]string{
	"ParentType":          "4",
	"ParentName":          "C_CONTENT",
	"ParentModule":        "BR_FLIB",
	"VisualAttributeName": "NORMAL_CANVAS",
	"ParentFilename":      "BR_FLIB.fmb",
	"ParentModuleType":    "12",
}

// stacked canvas -> C_CONTENT
func (P *FormsXMLProcessor) fixStackedCanvas(st *xml.StartElement) {
	if st.Name.Local != "Canvas" {
		return
	}
	name := getAttr(st.Attr, "Name")
	if getAttr(st.Attr, "CanvasType") == "Stacked" {
		st.Attr = st.Attr[:1]
		st.Attr[0].Name = xml.Name{Local: "Name"}
		st.Attr[0].Value = name
		for k, v := range stackedCanvasAttrs {
			st.Attr = append(st.Attr, xml.Attr{Name: xml.Name{Local: k}, Value: v})
		}
	} else {
		for i := len(st.Attr) - 1; i >= 0; i-- {
			a := st.Attr[i]
			if a.Name.Local == "Name" || stackedCanvasAttrs[a.Name.Local] == "" {
				continue
			}
			st.Attr = append(st.Attr[:i], st.Attr[i+1:]...)
		}
	}

	if getAttr(st.Attr, "Name") == "C_CONTENT" {
		st.Attr = setAttrs(st.Attr, DefaultContentCanvasAttrs)
	}
}

var DefaultContentCanvasAttrs = map[string]string{
	"VisualAttributeName": "NORMAL_CANVAS",
	"Width":               "1010", "Height": "621",
	"ViewportWidth": "720", "ViewportHeight": "432",
}

var coordinate = map[string]string{
	"CharacterCellWidth":  "9",
	"CharacterCellHeight": "18",
	"CoordinateSystem":    "Real",
	"RealUnit":            "Pixel",
	"DefaultFontScaling":  "false",
}

var DefaultVA = VisualAttribute{
	ParentModule: "BR_FLIB", ParentModuleType: "12",
	ParentFilename: "BR_FLIB.fmb", ParentType: "39",
	Attributes: []xml.Attr{{Name: xml.Name{Local: "DirtyInfo"}, Value: "true"}},
}

func (P *FormsXMLProcessor) fixCoordinate(st *xml.StartElement) {
	if st.Name.Local == "FormModule" {
		st.Attr = setAttr(st.Attr, "ConsoleWindow", "W_MAIN")
		st.Attr = setAttr(st.Attr, "MenuModule", "M_MENU")
		return
	}
	if st.Name.Local != "Coordinate" {
		return
	}
	st.Attr = setAttrs(st.Attr, coordinate)
}

// a használt, de nem létező VisualAttribute-okat subclassolja a BR_FLIB-ből
func (P *FormsXMLProcessor) addMissingVAs(enc *xml.Encoder, missing map[string]struct{}) error {
	for name := range missing {
		va := DefaultVA
		va.Name, va.ParentName = name, name
		if err := enc.Encode(va); err != nil {
			return err
		}
	}
	return nil
}

var RequiredParams = []string{"TORZSSZAM", "PRG_AZON", "BAZON", "DAZON"}

var RequiredParam = ModuleParameter{
	ParentType: "13", ParentModule: "BR_FLIB", ParentFilename: "BR_FLIB.fmb", ParentModuleType: "12",
}

// beteszi a 4 kötelező paramétert
func (P *FormsXMLProcessor) addMissingParams(enc *xml.Encoder, missing map[string]struct{}) error {
	for name := range missing {
		param := RequiredParam
		param.Name, param.ParentName = name, name
		if err := enc.Encode(param); err != nil {
			return err
		}
	}
	return nil
}

func (P *FormsXMLProcessor) scaleElt(st *xml.StartElement) {
	if st.Name.Local == "Coordinate" {
		return // against double scale
	}
	for i, a := range st.Attr {
		k := a.Name.Local
		isWidth := strings.HasSuffix(k, "Width")
		if isWidth || strings.HasSuffix(k, "Position") ||
			strings.HasSuffix(k, "Height") ||
			k == "DistanceBetweenRecords" {
			if val, _ := strconv.Atoi(a.Value); val > 0 {
				if isWidth || strings.HasSuffix(k, "XPosition") {
					val *= int(P.CellWidth)
				} else {
					val *= int(P.CellHeight)
				}
				a.Value = strconv.Itoa(val)
				st.Attr[i] = a
			}
		}
	}
}

type Subclass struct {
	ParentModule, ParentFilename string
}

var ParentModules = map[string]Subclass{
	"G_LIB":   Subclass{ParentModule: "BR_FLIB", ParentFilename: "BR_FLIB.fmb"},
	"CIM_LIB": Subclass{ParentModule: "BR_CIM_LIB", ParentFilename: "BR_CIM_LIB.fmb"},
}

func (P *FormsXMLProcessor) fixParentModule(st *xml.StartElement) {
	module := getAttr(st.Attr, "ParentModule")
	if pm, ok := ParentModules[module]; ok {
		st.Attr = setAttr(st.Attr, "ParentModule", pm.ParentModule)
		st.Attr = setAttr(st.Attr, "ParentFilename", pm.ParentFilename)
	}
}

var RootwindowSet = map[string]string{
	"ParentModule":        "BR_FLIB",
	"ParentName":          "W_MAIN",
	"ParentFilename":      "BR_FLIB.fmb",
	"ParentModuleType":    "12",
	"ParentType":          "41",
	"VisualAttributeName": "NORMAL",
	"Name":                "W_MAIN",
	//<Window Name="W_MAIN" ShowHorizontalScrollbar="false" MinimizeAllowed="false" Width="1010" ResizeAllowed="false" PrimaryCanvas="C_CONTENT" XPosition="0" YPosition="20" MaximizeAllowed="false" DirtyInfo="true" VisualAttributeName="NORMAL" Modal="true" MoveAllowed="false" ShowVerticalScrollbar="false" Height="601"/>
	"ShowHorizontalScrollbar": "false", "ShowVerticalScrollbar": "false",
	"MinimizeAllowed": "false", "MoveAllowed": "false", "ResizeAllowed": "false",
	//"PrimaryCanvas": "C_CONTENT", // Ne írj - KL 2019-03-20
	"Width": "1010", "Height": "601",
	"XPosition": "0", "YPosition": "0",
}
var RootwindowDel = []string{
	//"Height", "Width",
	"WindowStyle", "CloseAllowed",
	//"MoveAllowed", "ResizeAllowed", "MinimizeAllowed",
	"InheritMenu", "Bevel", "FontName", "FontSize",
	"FontWeight", "FontStyle", "FontSpacing",
}

/*
var ContentCanvasSet = map[string]string{
	//<Canvas Name="C_CONTENT" DirtyInfo="true" VisualAttributeName="NORMAL_CANVAS" Width="1010" WindowName="W_MAIN" Height="621" ViewportHeight="432" ViewportWidth="720"/>
	"VisualAttributeName": "NORMAL_CANVAS",
	"Width":               "1010", "Height": "610",
	"ViewportHeight": "432", "ViewportWidth": "720",
}
*/

func (P *FormsXMLProcessor) subclassRootwindow(st *xml.StartElement) {
	if i := findAttr(st.Attr, "WindowName"); i >= 0 && st.Attr[i].Value == "ROOT_WINDOW" {
		st.Attr[i].Value = "W_MAIN"
	}

	if st.Name.Local != "Window" || getAttr(st.Attr, "Name") != "ROOT_WINDOW" {
		return
	}
	m := make(map[string]struct{}, len(RootwindowDel))
	for _, d := range RootwindowDel {
		m[d] = struct{}{}
	}
	for i := len(st.Attr) - 1; i >= 0; i-- {
		k := st.Attr[i].Name.Local
		if _, ok := m[k]; ok {
			st.Attr = append(st.Attr[:i], st.Attr[i+1:]...)
		}
	}
	st.Attr = setAttrs(st.Attr, RootwindowSet)
}

var RequiredLibs = []string{"BR_PROCEDURE_LIB"}

type AttachedLibrary struct {
	Name            string `xml:",attr"`
	LibrarySource   string `xml:",attr,omitempty"`
	LibraryLocation string `xml:",attr"`
}

func (P *FormsXMLProcessor) attachLibs(enc *xml.Encoder) error {
	for _, lib := range RequiredLibs {
		if err := enc.Encode(AttachedLibrary{
			LibrarySource: "File", Name: lib, LibraryLocation: lib},
		); err != nil {
			return err
		}
	}
	return nil
}

var BadItemType = map[string]string{
	"Check Box":                  "Display Item",
	"User Area":                  "Text Item",
	"ActiveX Control (Obsolete)": "Text Item",
}

func (P *FormsXMLProcessor) fixBadItemType(st *xml.StartElement) {
	if st.Name.Local != "Item" {
		return
	}
	if i := findAttr(st.Attr, "ItemType"); i >= 0 {
		//log.Printf("ItemType[%d]=%q => %q", i, st.Attr[i].Value, BadItemType[st.Attr[i].Value])
		if v := BadItemType[st.Attr[i].Value]; v != "" {
			st.Attr[i].Value = v
		}
	}
}

var rSpaces = regexp.MustCompile(`\s+&amp;#10;`)

func (P *FormsXMLProcessor) trimSpaces(st *xml.StartElement) {
	switch st.Name.Local {
	case "ProgramUnit", "Trigger":
	default:
		return
	}
	if i := findAttr(st.Attr, st.Name.Local+"Text"); i >= 0 && st.Attr[i].Value != "" {
		st.Attr[i].Value = rSpaces.ReplaceAllString(st.Attr[i].Value, "&amp;#10;")
	}
}

var VisualAttrs = map[string]string{
	"ParentModule": "BR_FLIB", "ParentModuleType": "12",
	"ParentFileName": "BR_FLIB.fmb",
}
var VAReplace = map[string]struct {
	Replacement string
	Names       []string
}{
	"ITEM_SELECT": {
		Replacement: "SELECT",
		Names:       []string{"RecordVisualAttributeGroupName", "VisualAttributeGroupName"},
	},
}

func (P *FormsXMLProcessor) fixVAs(st *xml.StartElement) {
	for rnev, R := range VAReplace {
		for _, k := range R.Names {
			i := findAttr(st.Attr, k)
			if i < 0 {
				continue
			}
			if st.Attr[i].Value != rnev {
				continue
			}
			if R.Replacement == "" {
				st.Attr = append(st.Attr[:i], st.Attr[i+1:]...)
			} else {
				P.missingVAs[R.Replacement] = struct{}{}
				st.Attr[i].Value = R.Replacement
			}
		}
	}

	switch st.Name.Local {
	case "Block":
		st.Attr = setAttr(st.Attr, "RecordVisualAttributeGroupName", "NORMAL")

	case "NORMAL":
		i := findAttr(st.Attr, "Name")
		rnev := st.Attr[i].Value
		if R, ok := VAReplace[rnev]; ok {
			st.Attr[i].Value = R.Replacement
			st.Attr = setAttr(st.Attr, "ParentName", R.Replacement)
			P.missingVAs[R.Replacement] = struct{}{}
		}
		if i = findAttr(st.Attr, "ParentModule"); i >= 0 && st.Attr[i].Value == "G_LIB" {
			st.Attr = setAttrs(st.Attr, VisualAttrs)
		}
	}
}

var RemovePromptVAs = []string{
	"PromptFontName", "PromptFontSize", "PromptFontSpacing",
	"PromptFontStyle", "PromptFontWeight",
}
var RemoveVAs = []string{"FontName", "FontSize", "FontSpacing", "FontStyle", "FontWeight"}

// a PromptVisualAttribute-ot beállítja NORMAL_PROMPT-ra
func (P *FormsXMLProcessor) fixPromptVAs(st *xml.StartElement) {
	if i := findAttr(st.Attr, "Prompt"); i >= 0 {
		if j := findAttr(st.Attr, "PromptVisualAttributeName"); j < 0 || st.Attr[j].Value == "DEFAULT" {
			if j >= 0 {
				st.Attr[j].Value = "NORMAL_PROMPT"
			}
			if P.tbdPromptVAs == nil {
				P.tbdPromptVAs = make(map[string]struct{}, len(RemovePromptVAs))
				for _, k := range RemovePromptVAs {
					P.tbdPromptVAs[k] = struct{}{}
				}
			}
			delAttrs(st.Attr, P.tbdPromptVAs)
			P.missingVAs["NORMAL_PROMPT"] = struct{}{}
		}
	}
	if j := findAttr(st.Attr, "VisualAttributeName"); j >= 0 {
		if P.tbdVAs == nil {
			P.tbdVAs = make(map[string]struct{}, len(RemoveVAs))
			for _, k := range RemoveVAs {
				P.tbdVAs[k] = struct{}{}
			}
		}
		delAttrs(st.Attr, P.tbdVAs)
	}
}

func findAttr(attrs []xml.Attr, name string) int {
	for i, a := range attrs {
		if a.Name.Local == name {
			return i
		}
	}
	return -1
}

func getAttr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func setAttr(attrs []xml.Attr, k, v string) []xml.Attr {
	for i, a := range attrs {
		if a.Name.Local == k {
			a.Value = v
			attrs[i] = a
			return attrs
		}
	}
	return append(attrs, xml.Attr{Name: xml.Name{Local: k}, Value: v})
}

func setAttrs(attrs []xml.Attr, m map[string]string) []xml.Attr {
	seen := make(map[string]struct{})
	for i, a := range attrs {
		if v := m[a.Name.Local]; v != "" {
			a.Value = v
			attrs[i] = a
			seen[a.Name.Local] = struct{}{}
		}
	}
	for k, v := range m {
		if _, ok := seen[k]; ok {
			continue
		}
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: k}, Value: v})
	}
	return attrs
}
func delAttrs(attrs []xml.Attr, m map[string]struct{}) []xml.Attr {
	for i := len(attrs) - 1; i >= 0; i-- {
		a := attrs[i]
		if _, ok := m[a.Name.Local]; ok {
			attrs = append(attrs[:i], attrs[i+1:]...)
		}
	}
	return attrs
}

func fixAttrs(attrs []xml.Attr) []xml.Attr {
	seen := make(map[string]struct{}, len(attrs))
	for i := len(attrs) - 1; i >= 0; i-- {
		nm := attrs[i].Name.Local
		if _, ok := seen[nm]; ok {
			attrs = append(attrs[:i], attrs[i+1:]...)
			continue
		}
		seen[nm] = struct{}{}
	}
	return attrs
}

type VisualAttribute struct {
	Name             string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	ParentType       string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
}

type ModuleParameter struct {
	Name             string     `xml:",attr,omitempty"`
	ParentType       string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
}

/*
type Module struct {
	Version    string     `xml:"version,attr"`
	Attributes []xml.Attr `xml:",any,attr"`
	FormModule FormModule
}
type FormModule struct {
	Name             string            `xml:"Name,attr"`
	ConsoleWindow    string            `xml:",attr"`
	Attributes       []xml.Attr        `xml:",any,attr"`
	Coordinate       []Coordinate      `xml:"Coordinate"`
	Alerts           []Alert           `xml:"Alert"`
	Blocks           []Block           `xml:"Block"`
	Canvases         []Canvas          `xml:"Canvas"`
	Params           []ModuleParameter `xml:"ModuleParameter"`
	LOVs             []LOV             `xml:"LOV"`
	ProgramUnits     []ProgramUnit     `xml:"ProgramUnit"`
	PropertyClasses  []PropertyClass   `xml:"PropertyClass"`
	RecordGroups     []RecordGroup     `xml:"RecordGroup"`
	Triggers         []Trigger         `xml:"Trigger"`
	VisualAttributes []VisualAttribute `xml:"VisualAttribute"`
	Windows          []Window          `xml:"Window"`
}
type Coordinate struct {
	CharacterCellWidth  string     `xml:",attr,omitempty"`
	RealUnit            string     `xml:",attr,omitempty"`
	DefaultFontScaling  string     `xml:",attr,omitempty"`
	CharacterCellHeight string     `xml:",attr,omitempty"`
	CoordinateSystem    string     `xml:",attr,omitempty"`
	Attributes          []xml.Attr `xml:",any,attr"`
}

type Alert struct {
	Name       string     `xml:",attr,omitempty"`
	Attributes []xml.Attr `xml:",any,attr"`
}

type Block struct {
	Name              string             `xml:",attr,omitempty"`
	ScrollbarWidth    string             `xml:",attr,omitempty"`
	ScrollbarLength   string             `xml:",attr,omitempty"`
	Attributes        []xml.Attr         `xml:",any,attr"`
	Items             []Item             `xml:"Item"`
	DataSourceColumns []DataSourceColumn `xml:"DataSourceColumn"`
	Triggers          []Trigger          `xml:"Trigger"`
}

type Item struct {
	Name                      string        `xml:",attr,omitempty"`
	XPosition                 string        `xml:",attr,omitempty"`
	YPosition                 string        `xml:",attr,omitempty"`
	Width                     string        `xml:",attr,omitempty"`
	Height                    string        `xml:",attr,omitempty"`
	PromptAttachmentOffset    string        `xml:",attr,omitempty"`
	PromptAlignOffset         string        `xml:",attr,omitempty"`
	PromptForegroundColor     string        `xml:",attr,omitempty"`
	VisualAttributeName       string        `xml:",attr,omitempty"`
	PromptVisualAttributeName string        `xml:",attr,omitempty"`
	Attributes                []xml.Attr    `xml:",any,attr"`
	RadioButtons              []RadioButton `xml:"RadioButton"`
	Triggers                  []Trigger     `xml:"Trigger"`
}

type DataSourceColumn struct {
	DSCName    string     `xml:",attr"`
	Type       string     `xml:",attr"`
	Attributes []xml.Attr `xml:",any,attr"`
}

type RadioButton struct {
	Name                      string     `xml:",attr,omitempty"`
	BackColor                 string     `xml:",attr,omitempty"`
	XPosition                 string     `xml:",attr,omitempty"`
	YPosition                 string     `xml:",attr,omitempty"`
	FontName                  string     `xml:",attr,omitempty"`
	VisualAttributeName       string     `xml:",attr,omitempty"`
	PromptVisualAttributeName string     `xml:",attr,omitempty"`
	Height                    string     `xml:",attr,omitempty"`
	Width                     string     `xml:",attr,omitempty"`
	Attributes                []xml.Attr `xml:",any,attr"`
}

type Canvas struct {
	Name                string     `xml:",attr,omitempty"`
	CanvasType          string     `xml:",attr,omitempty"`
	ParentType          string     `xml:",attr,omitempty"`
	ParentName          string     `xml:",attr,omitempty"`
	ParentModule        string     `xml:",attr,omitempty"`
	ParentFilename      string     `xml:",attr,omitempty"`
	ParentModuleType    string     `xml:",attr,omitempty"`
	VisualAttributeName string     `xml:",attr,omitempty"`
	Width               string     `xml:",attr,omitempty"`
	WindowName          string     `xml:",attr,omitempty"`
	Height              string     `xml:",attr,omitempty"`
	ViewportHeight      string     `xml:",attr,omitempty"`
	ViewportWidth       string     `xml:",attr,omitempty"`
	Attributes          []xml.Attr `xml:",any,attr"`
	Graphics            []Graphics `xml:"Graphics,omitempty"`
	TabPages            []TabPage  `xml:"TabPage,omitempty"`
}

type TabPage struct {
	Name                string     `xml:",attr,omitempty"`
	VisualAttributeName string     `xml:",attr,omitempty"`
	Attributes          []xml.Attr `xml:",any,attr"`
	Graphics            []Graphics `xml:"Graphics,omitempty"`
}
type Graphics struct {
	Name                          string     `xml:",attr,omitempty"`
	InternalLineWidth             string     `xml:",attr,omitempty"`
	Width                         string     `xml:",attr,omitempty"`
	EdgeForegroundColor           string     `xml:",attr,omitempty"`
	EdgeBackColor                 string     `xml:",attr,omitempty"`
	FrameTitleVisualAttributeName string     `xml:",attr,omitempty"`
	EdgePattern                   string     `xml:",attr,omitempty"`
	FillPattern                   string     `xml:",attr,omitempty"`
	FrameTitleForegroundColor     string     `xml:",attr,omitempty"`
	FrameTitleFontSize            string     `xml:",attr,omitempty"`
	FrameTitleFontStyle           string     `xml:",attr,omitempty"`
	FrameTitleFontName            string     `xml:",attr,omitempty"`
	FrameTitleFontWeight          string     `xml:",attr,omitempty"`
	FrameTitleFontSpacing         string     `xml:",attr,omitempty"`
	FrameTitleOffset              string     `xml:",attr,omitempty"`
	FrameTitleSpacing             string     `xml:",attr,omitempty"`
	GraphicsFontName              string     `xml:",attr,omitempty"`
	GraphicsFontSize              string     `xml:",attr,omitempty"`
	GraphicsFontStyle             string     `xml:",attr,omitempty"`
	BackColor                     string     `xml:",attr,omitempty"`
	XPosition                     string     `xml:",attr,omitempty"`
	YPosition                     string     `xml:",attr,omitempty"`
	GraphicsFontWeight            string     `xml:",attr,omitempty"`
	GraphicsFontColor             string     `xml:",attr,omitempty"`
	GraphicsFontSpacing           string     `xml:",attr,omitempty"`
	GraphicsFontColorCode         string     `xml:",attr,omitempty"`
	HorizontalMargin              string     `xml:",attr,omitempty"`
	ScrollbarWidth                string     `xml:",attr,omitempty"`
	VerticalMargin                string     `xml:",attr,omitempty"`
	HorizontalObjectOffset        string     `xml:",attr,omitempty"`
	Height                        string     `xml:",attr,omitempty"`
	StartPromptOffset             string     `xml:",attr,omitempty"`
	VisualAttributeName           string     `xml:",attr,omitempty"`
	WindowName                    string     `xml:",attr,omitempty"`
	ViewportHeight                string     `xml:",attr,omitempty"`
	ViewportWidth                 string     `xml:",attr,omitempty"`
	Attributes                    []xml.Attr `xml:",any,attr"`
}

type LOV struct {
	Name                string             `xml:",attr,omitempty"`
	VisualAttributeName string             `xml:",attr,omitempty"`
	Width               string             `xml:",attr,omitempty"`
	Height              string             `xml:",attr,omitempty"`
	Attributes          []xml.Attr         `xml:",any,attr"`
	Mapping             []LOVColumnMapping `xml:"LOVColumnMapping"`
}

type LOVColumnMapping struct {
	Name         string     `xml:",attr,omitempty"`
	DisplayWidth string     `xml:",attr,omitempty"`
	Attributes   []xml.Attr `xml:",any,attr"`
}

type ProgramUnit struct {
	Name            string     `xml:",attr,omitempty"`
	ProgramUnitText string     `xml:",attr,omitempty"`
	ProgramUnitType string     `xml:",attr,omitempty"`
	Attributes      []xml.Attr `xml:",any,attr"`
}

type PropertyClass struct {
	Name             string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	ParentType       string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
}

type RecordGroup struct {
	Name             string              `xml:",attr,omitempty"`
	RecordGroupQuery string              `xml:",attr,omitempty"`
	Attributes       []xml.Attr          `xml:",any,attr"`
	Columns          []RecordGroupColumn `xml:"RecordGroupColumn"`
}
type RecordGroupColumn struct {
	Name       string     `xml:",attr,omitempty"`
	Attributes []xml.Attr `xml:",any,attr"`
}

type Window struct {
	Name                string     `xml:",attr,omitempty"`
	ParentModule        string     `xml:",attr,omitempty"`
	ParentModuleType    string     `xml:",attr,omitempty"`
	ParentName          string     `xml:",attr,omitempty"`
	ParentFilename      string     `xml:",attr,omitempty"`
	ParentType          string     `xml:",attr,omitempty"`
	VisualAttributeName string     `xml:",attr,omitempty"`
	XPosition           string     `xml:",attr,omitempty"`
	YPosition           string     `xml:",attr,omitempty"`
	Width               string     `xml:",attr,omitempty"`
	Height              string     `xml:",attr,omitempty"`
	Attributes          []xml.Attr `xml:",any,attr"`
}

func (P *FormsXMLProcessor) Parse(dec *xml.Decoder) error {
	if P.CellWidth == 0 {
		P.CellWidth = DefaultCellWidth
	}
	if P.CellHeight == 0 {
		P.CellHeight = DefaultCellHeight
	}
	if P.UsedVisualAttributes == nil {
		P.UsedVisualAttributes = make(map[string]struct{}, len(DefaultUsedVisualAttributes))
		for _, a := range DefaultUsedVisualAttributes {
			P.UsedVisualAttributes[a] = struct{}{}
		}
	}

	err := dec.Decode(&P.Module)
	return err
}
*/

/*
	# TODO: !
    # 1. Form.Physical.Coordinate System nél a systemet pixel-re
    # 					a width-et 12 -re
    # 					a height-et 24 -ra álltíani
    #     Form.Functional.Console Window-t W_main-re állítani
    # 2. BR_FLIB-ből átmásolni a C_CONTENT-et a Canvases-ba
    # 3. A mezőknél a Physical.Canvas-t  átírni C_CONTENT-re
    # 4. BR_FLIB-ből a
    # NORMAL_ITEM
    # SELECT
    # NORMAL_PROMPT
    # NORMAL
    # Visual Attributes-ban lévő atribútumokat áthúzni
    # 5. A BLOCK-oknál a Record.Current Record Visual Attribute Group-ot SELECT-re váltani
    # 6. A mezőknél a Visual Attributes-ban :
    # A Visual Attriburte Group : NORMAL_ITEM r állítani
    # A Prompt Visual Attribute Group :  NORMAL_PROMPT ra állítani
    # 7. WINDOW-ba inheritálni kell egy csomó attribútumot (miket ?)
    # 8. Mező szélesség kb. 10x hossz + 6
    # 9. Ha a Canvas-on a mezők kerete nem látszik a mező property
    # palette-en :
    # Physical.Bevel : Lowered re
    # Physical.Rendered YES -re
*/

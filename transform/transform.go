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

package transform

import (
	"encoding/xml"
	"io"
)

const DefaultCellWidth, DefaultCellHeight = 12, 24

var DefaultUsedVisualAttributes = []string{"NORMAL_ITEM", "SELECT", "NORMAL_PROMPT", "NORMAL"}

type FormsXMLProcessor struct {
	CellWidth, CellHeight uint8
	UsedVisualAttributes  map[string]struct{}
	UnknownParents        map[string]struct{}

	Module Module
}

type Module struct {
	Version    string     `xml:"version,attr"`
	Attributes []xml.Attr `xml:",any,attr"`
	FormModule FormModule
}
type FormModule struct {
	Name             string     `xml:"Name,attr"`
	Attributes       []xml.Attr `xml:",any,attr"`
	Coordinate       Coordinate
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

type Canvas struct {
	Name                string     `xml:",attr,omitempty"`
	CanvasType          string     `xml:",attr,omitempty"`
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

type ModuleParameter struct {
	Name             string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
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

type VisualAttribute struct {
	Name             string     `xml:",attr,omitempty"`
	ParentModule     string     `xml:",attr,omitempty"`
	ParentModuleType string     `xml:",attr,omitempty"`
	ParentName       string     `xml:",attr,omitempty"`
	ParentFilename   string     `xml:",attr,omitempty"`
	ParentType       string     `xml:",attr,omitempty"`
	Attributes       []xml.Attr `xml:",any,attr"`
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

func (P *FormsXMLProcessor) Process() error {
	return nil
}
func (P *FormsXMLProcessor) Write(w io.Writer) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(P.Module)
}

/*
    def write(self, tgt):
        assert self.dom is not None
        if not hasattr(tgt, 'write'): tgt = file(tgt, 'w+')
        self.dom.writexml(tgt)

    def parse(self):
        self.dom = parsexml(self.fn)
        self.module = self.dom.documentElement.getElementsByTagName('FormModule')[0]
        self.module_name = self.module.getAttribute('Name')
        self.module_version = self.module.getAttribute('RuntimeComp')
        # print self.module_name
        return self.dom

    def run(self):
        if not self.dom: self.parse()
        self._removeExcess(self.module)
        self._canvasStacked(self.module)
        self._coordinate(self.module)
        self._traverse(self.module)
        self._missingVAs(self.module)
        self._ensureParameters(self.module)

    def _removeExcess(self, doc):
        u'''Törli a felesleges elemeket'''
        for node in doc.getElementsByTagName('Alert'):
            if node.getAttribute('Name') in ('KERDEZ_ALERT', 'UZEN_ALERT'):
                doc.removeChild(node)

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

    c_stacked_canvas_d = {'ParentType': '4', 'ParentName': 'C_STCK_CONTENT',
        'ParentModule': 'BR_FLIB', 'VisualAttributeName': 'NORMAL',
        'ParentFilename': 'BR_FLIB.fmb', 'ParentModuleType': '12'}
    def _canvasStacked(self, doc):
        u'''stacked canvas -> C_CONTENT'''
        for node in doc.getElementsByTagName('Canvas'):
            if 'Stacked' == node.getAttribute('CanvasType'):
                for k, v in self.c_stacked_canvas_d.iteritems():
                    node.setAttribute(k, v)
            # minden inherited
            removable = (set(self._attr_names(node))
                - (set(self.c_stacked_canvas_d.iterkeys()) | set(['Name'])))
            # print removable
            for k in removable:
                node.removeAttribute(k)

    c_coordinate_d = {'CharacterCellWidth': '9',
        'CharacterCellHeight': '18',
        'CoordinateSystem': 'Real',
        'RealUnit': 'Pixel',
        'DefaultFontScaling': 'false'}
    def _coordinate(self, doc):
        self.module.setAttribute('ConsoleWindow', 'W_MAIN')
        for node in doc.getElementsByTagName("Coordinate"):
            for k, v in self.c_coordinate_d.iteritems(): node.setAttribute(k, v)

    def _walk(self, elt):
        alist = [elt]
        while alist:
            elt = alist.pop()
            yield elt
            if elt.hasChildNodes(): alist = list(elt.childNodes) + alist
            # print [(hasattr(x, 'tagName') and [x.tagName] or [x.data])[0] for x in alist]
            #DFS

    def _traverse(self, elt):
        for node in self._walk(elt):
            if not node.nodeType == node.ELEMENT_NODE:
                continue
            # print node.nodeType, node.nodeName, node.nodeValue
            self._scaleElt(node, self.c_cell_width, self.c_cell_height)
            self._subclassingElt(node)
            self._rootwindowElt(node)
            self._attachLibsElt(node)
            #LOG.debug("module_version: %r" % self.module_version)
            if self.module_version >= '4.5':
                self._badItemType(node)
            self._trimSpaces(node)
            self._visualAttributeElt(node)
            self._promptVAElt(node)

    def _attr_names(self, elt):
        for i in xrange(0, elt.attributes.length):
            yield elt.attributes.item(i).name

    def _scaleElt(self, elt, width, height):
        if elt.tagName == 'Coordinate': return # dupla szorzás ellen
        for key in self._attr_names(elt):
            # print elt.nodeName
            if (key.endswith("Position") or key.endswith("Width")
                    or key.endswith("Height") or "DistanceBetweenRecords" == key):
                # print '_scaleElt', elt.nodeName
                val = -1
                try: val = int(elt.getAttribute(key))
                except ValueError: pass
                if val > 0:
                    if key.endswith("Width") or key.endswith("XPosition"):
                        val *= width
                    else: val *= height
                    #System.out.println(key + ": " + attr.getValue() + " -> " + val);
                    elt.setAttribute(key, str(val))

    c_sc_parent_type_d = {'Trigger': '37', 'Window': '41',
            'VisualAttribute': '39',
        }
    def _sc_set_parent_type(self, elt):
        if self.c_sc_parent_type_d.has_key(elt.tagName):
            elt.setAttribute('ParentType', self.c_sc_parent_type_d[elt.tagName])

    c_sc_d = {
        'G_LIB': {
            'ParentModule': "BR_FLIB",
            'ParentFilename': "BR_FLIB.fmb"},
        'CIM_LIB': {
            'ParentModule': "BR_CIM_LIB",
            'ParentFilename': "BR_CIM_LIB.fmb"},
         }
    def _subclassingElt(self, elt):
        if elt.hasAttribute("ParentModule"):
            module = elt.getAttribute("ParentModule")
            if (not self.module_name == module
                    and self.c_sc_d.has_key(module)):
                for k, v in self.c_sc_d[module].iteritems():
                    # print '%s.%s = %s -> %s' % (module, k, elt.getAttribute(k), v)
                    elt.setAttribute(k, v)
                self._sc_set_parent_type(elt)

            elif not (self.module_name == module
                                or self.c_sc_d.has_key(module)):
                self.unknown_parents.add(module)
        # else: print list(self._attr_names(elt))

    c_rootwindow_d = {"ParentModule": "BR_FLIB",
        "ParentName": "W_MAIN",
        "ParentFilename": "BR_FLIB.fmb",
        "ParentModuleType": "12",
        'VisualAttributeName': 'NORMAL',
        "Name": "W_MAIN"}
    c_rootwindow_del = ('Height', 'Width', 'WindowStyle', 'CloseAllowed',
                                            'MoveAllowed', 'ResizeAllowed', 'MinimizeAllowed',
                                            'InheritMenu', 'Bevel', 'FontName', 'FontSize',
                                            'FontWeight', 'FontStyle', 'FontSpacing')
    ##
    #  a ROOT_WINDOW-t subclass-olja a W_main-ről
    def _rootwindowElt(self, elt):
        # ROOT_WINDOW -> W_MAIN
        if "Window" == elt.tagName and "ROOT_WINDOW" == elt.getAttribute("Name"):
            self._sc_set_parent_type(elt)
            for k, v in self.c_rootwindow_d.iteritems(): elt.setAttribute(k, v)
            for k in self.c_rootwindow_del:
                if elt.hasAttribute(k): elt.removeAttribute(k)
        # ahol hivatkoznak rájuk, ott is
        for k in ('WindowName',):
            if elt.hasAttribute(k) and elt.getAttribute(k) == 'ROOT_WINDOW':
                elt.setAttribute(k, 'W_MAIN')


    c_attachLibs = set(['BR_PROCEDURE_LIB',])
    def _attachLibsElt(self, elt):
        if 'FormModule' == elt.tagName:
            if len(elt.getElementsByTagName("AttachedLibrary")) == 0:
                #hova tegyük
                ele = None
                for e in elt.childNodes:
                    if not e.nodeType == e.ELEMENT_NODE:
                        continue
                    # print e, e.nodeName
                    if 'Block' == e.tagName:
                        ele = e
                        break
                if not ele: return
                #ha nincs
                for lib_name in self.c_attachLibs:
                    uj = self.dom.createElement("AttachedLibrary")
                    uj.setAttribute('LibrarySource', 'File')
                    for k in ('Name', 'LibraryLocation'):
                        uj.setAttribute(k, lib_name)
                elt.insertBefore(uj, ele)

    c_badItemType = {'Check Box': 'Display Item', 'User Area': 'Text Item'}
    def _badItemType(self, elt):
        if 'Item' == elt.tagName:
            k = elt.getAttribute('ItemType')
            if k in self.c_badItemType:
                elt.setAttribute('ItemType', self.c_badItemType[k])

    r_spaces = re.compile(r'\s+&amp;#10;')
    def _trimSpaces(self, elt):
        if elt.tagName in ('ProgramUnit', 'Trigger'):
            k = elt.tagName + 'Text'
            v = elt.getAttribute(k)
            if v:
                elt.setAttribute(k, self.r_spaces.sub('&amp;#10;', v))

    c_visualAttribs_d = {'ParentModule': 'BR_FLIB', 'ParentModuleType': '12',
            'ParentFileName': 'BR_FLIB.fmb'}
    c_VAs2replace_d = {
        'ITEM_SELECT': ('SELECT', set(['RecordVisualAttributeGroupName',
                                                                     'VisualAttributeGroupName'])),}
    def _visualAttributeElt(self, elt):
        # használt VA-k
        for rnev, (unev, elt_names) in self.c_VAs2replace_d.iteritems():
            for k in elt_names:
                if elt.hasAttribute(k) and elt.getAttribute(k) == rnev:
                    print elt.getAttribute('Name'), k, rnev, '->', unev
                    if unev is None: elt.removeAttribute(k)
                    else:
                        self.visual_attributes['used'].add(unev)
                        elt.setAttribute(k, unev)
        if 'Block' == elt.tagName:
            elt.setAttribute('RecordVisualAttributeGroupName', 'SELECT')
        # subclassing
        if 'VisualAttribute' == elt.tagName:
            rnev = elt.getAttribute('Name')
            if self.c_VAs2replace_d.has_key(rnev):
                unev = self.c_VAs2replace_d[rnev][0]
                elt.setAttribute('Name', unev)
                # más a subclassing név!!!
                elt.setAttribute('ParentName', unev)
            self.visual_attributes['exists'].add(unev)
            self._sc_set_parent_type(elt)
            if elt.getAttribute('ParentModule') == 'G_LIB':
                for k, v in self.c_visualAttribs_d.iteritems(): elt.setAttribute(k, v)

    def _missingVAs(self, elt):
        u'''a használt, de nem létező VisualAttribute-okat subclassolja a BR_FLIB-ből'''
        # print 'used:', self.visual_attributes['used'], '\nexists:', self.visual_attributes['exists']
        needed = self.visual_attributes['used'] - self.visual_attributes['exists']
        # print 'MissingVAs:', self.visual_attributes['used'], self.visual_attributes['exists'], needed
        if needed:
            # print elt.getElementsByTagName('Window')
            before = elt.getElementsByTagName('Window')[0]
            # print 'before =', before
            if not before: return
            for name in needed:
                uj = self.dom.createElement('VisualAttribute')
                for k, v in {'Name': name, 'ParentName': name, 'DirtyInfo': 'true'}.iteritems():
                    uj.setAttribute(k, v)
                self._sc_set_parent_type(uj)
                for k, v in self.c_visualAttribs_d.iteritems():
                    if k != 'ParentFileName':
                        uj.setAttribute(k, v)
                # print uj.toprettyxml()
                elt.insertBefore(uj, before)

    def _promptVAElt(self, elt):
        u'''a PromptVisualAttribute-ot beállítja NORMAL_PROMPT-ra'''
        if elt.hasAttribute('Prompt'):
            if (not elt.hasAttribute('PromptVisualAttributeName')
                    or 'DEFAULT' == elt.getAttribute('PromptVisualAttributeName')):
                elt.setAttribute('PromptVisualAttributeName', 'NORMAL_PROMPT')
                # print elt.toprettyxml()
                for k in ('PromptFontName', 'PromptFontSize', 'PromptFontSpacing',
                          'PromptFontStyle', 'PromptFontWeight',):
                    # print k, elt.getAttribute(k)
                    if elt.hasAttribute(k):
                        elt.removeAttribute(k)
                # print elt.toprettyxml()
                self.visual_attributes['used'].add('NORMAL_PROMPT')
        if elt.hasAttribute('VisualAttributeName'):
            for k in ('FontName', 'FontSize', 'FontSpacing', 'FontStyle', 'FontWeight'):
                if elt.hasAttribute(k): elt.removeAttribute(k)

    c_parameter = {'ParentType': '13', 'ParentModule': 'BR_FLIB',
                                 'ParentFilename': 'BR_FLIB.fmb', 'ParentModuleType': '12'}
    c_req_params = ('TORZSSZAM', 'PRG_AZON', 'BAZON', 'DAZON',)
    def _ensureParameters(self, elt):
        u'''beteszi a 4 kötelező paramétert'''

        exists = elt.getElementsByTagName('ModuleParameter')
        needed = set(self.c_req_params) - set(x.getAttribute('Name') for x in exists)
        # print exists, needed
        if needed: # Canvas után megy
            afters = []
            for nm in ('Canvas', 'Block'):
                afters = elt.getElementsByTagName(nm)
                if afters: break
            nxt = (afters[-1]).nextSibling
            while nxt.nodeType != nxt.ELEMENT_NODE:
                # print nxt.nodeName, nxt.nodeType
                nxt = nxt.nextSibling
            before = nxt
            # print 'BEFORE:', before.nodeName, before.nodeType
            for name in needed:
                # print name
                uj = self.dom.createElement('ModuleParameter')
                for k, v in self.c_parameter.iteritems(): uj.setAttribute(k, v)
                for k in ('Name', 'ParentName'): uj.setAttribute(k, name)
                # print uj.toprettyxml()
                elt.insertBefore(uj, before)


def dec(obj, encoding=ENCODING):
    if isinstance(obj, unicode):
        return obj
    elif hasattr(obj, '__unicode__'):
        return unicode(obj)
    else:
        return unicode(obj, encoding)

class EncodingWriter(file):
    u'''Adott fájlba ír, adott karakterkódolásokat használva'''
    def __init__(self, fn,
                             mode='w+b', input_encoding='utf-8', output_encoding='utf-8'):
        self._sup = super(type(self), self)
        self._sup.__init__(fn, mode=mode)
        self.inp_enc = input_encoding
        self.out_enc = output_encoding

    def write(self, text):
        if self.inp_enc and not isinstance(text, unicode):
            text = unicode(text, self.inp_enc)
        self._sup.write(text.encode(self.out_enc))
        self.flush()

def process_file(cmd, fn, out_fn, **kwds):
        P = FormsXmlProcessor(tmp_fn)

        P.run()
        for x in P.unknown_parents: unknown_parents.add(x)
        P.write(EncodingWriter(tmp_fn))
*/

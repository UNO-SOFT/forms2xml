#!/usr/bin/env python
# -*- coding: iso-8859-2 -*-
# :mode=python:encoding=ISO-8859-2:

import os, sys, optparse, shutil, glob, re, tempfile
from time import time
from itertools import islice
from pprint import pformat
import logging
LOG = logging.getLogger(__name__)

ENCODING = 'iso8859_2'
FIX_JAVA_PATH = [r'C:\oracle\mw11gR1\fr11gR2\jdk\jre\bin']

def get_jvm(default=None):
    sep = os.pathsep
    fn = (os.sys.platform.startswith('win') and ['java.exe'] or ['java'])[0]
    ret = default
    for dn in FIX_JAVA_PATH + os.environ['PATH'].split(sep):
        #print dn, os.path.join(dn, fn), os.path.exists(os.path.join(dn, fn))
        # if 'iDS' in dn: continue
        if os.path.exists(os.path.join(dn, fn)):
            ret = os.path.join(dn, fn)
            break
    print ret
    return '%s' % ret

c_java_d = {'jvm': get_jvm('jv.bat'), 'class': 'unosoft.forms.TransForm',
        'jar': 'transform.jar',
        'D': {'java.library.path': r'C:\oracle\mw11gR1\fr11gR2\BIN'},
        'cp': [
            '.', 
            r"C:\oracle\mw11gR1\fr11gR2\forms\java",
            r"C:\oracle\mw11gR1\fr11gR2\jlib",
            r"C:\oracle\mw11gR1\fr11gR2\jlib\frmjdapi.jar", 
            r"C:\oracle\mw11gR1\fr11gR2\jlib\frmxmltools.jar", 
            r"C:\oracle\mw11gR1\fr11gR2\lib\xmlparserv2.jar"
            ]
        }
print c_java_d
os.environ['PATH'] = c_java_d['D']['java.library.path'] + os.pathsep + os.environ['PATH']

JAVA_JUST_DUMP = True
JYTHON = 1
if not sys.platform.startswith('java'):
    JYTHON = 0
if JYTHON:
    if JYTHON == 2:
        import rule
        from TransForm import TransForm
    if JYTHON == 1:
        try:
            import unosoft.forms.TransForm as TransForm
        except:
            JYTHON = 0
else:
    from threading import Thread
    from time import sleep

GUI = False#True
options = None
if GUI:
    import wx
    class MainFrame(wx.Frame):
        def __init__(self, *args, **kwds):
            # begin wxGlade: MainFrame.__init__
            kwds["style"] = wx.DEFAULT_FRAME_STYLE
            wx.Frame.__init__(self, *args, **kwds)
            self.nb_opciok = wx.Notebook(self, -1, style=0)
            self.nb_opciok_pane_2 = wx.Panel(self.nb_opciok, -1)
            self.notebook_1_pane_1 = wx.Panel(self.nb_opciok, -1)
            self.sizer_2_staticbox = wx.StaticBox(self.notebook_1_pane_1, -1, u"Forrás")
            self.rb_src = wx.RadioBox(self.notebook_1_pane_1, -1, u"Forrás", choices=[u"fájl", u"könyvtár"], majorDimension=0, style=wx.RA_SPECIFY_ROWS)
            self.tc_src = wx.TextCtrl(self.notebook_1_pane_1, -1, "")
            self.bu_src = wx.Button(self.notebook_1_pane_1, -1, "...")
            self.cb_recurse = wx.CheckBox(self.notebook_1_pane_1, -1, u"rekurzív")
            self.bu_ok = wx.Button(self.notebook_1_pane_1, wx.ID_OK, "OK")
            self.bu_cancel = wx.Button(self.notebook_1_pane_1, wx.ID_CANCEL, u"Mégse")
            self.la_dsn = wx.StaticText(self.nb_opciok_pane_2, -1, "DSN")
            self.tc_dsn = wx.TextCtrl(self.nb_opciok_pane_2, -1, "")
            self.la_jar = wx.StaticText(self.nb_opciok_pane_2, -1, "jar")
            self.tc_jar = wx.TextCtrl(self.nb_opciok_pane_2, -1, "")
            self.la_workdir = wx.StaticText(self.nb_opciok_pane_2, -1, "work dir")
            self.tc_workdir = wx.TextCtrl(self.nb_opciok_pane_2, -1, "")
            self.la_threads = wx.StaticText(self.nb_opciok_pane_2, -1, "threads")
            self.sl_threads = wx.Slider(self.nb_opciok_pane_2, -1, 2, 1, 10, style=wx.SL_HORIZONTAL|wx.SL_AUTOTICKS|wx.SL_LABELS|wx.SL_LEFT|wx.SL_RIGHT)
            self.tc_log = wx.TextCtrl(self, -1, "", style=wx.TE_MULTILINE|wx.TE_READONLY)

            self.__set_properties()
            self.__do_layout()

            self.Bind(wx.EVT_RADIOBOX, self.rb_src_changed, self.rb_src)
            self.Bind(wx.EVT_BUTTON, self.bu_src_pressed, self.bu_src)
            self.Bind(wx.EVT_CHECKBOX, self.cb_recurse_checked, self.cb_recurse)
            self.Bind(wx.EVT_BUTTON, self.bu_ok_pressed, id=wx.ID_OK)
            self.Bind(wx.EVT_BUTTON, self.bu_cancel_pressed, id=wx.ID_CANCEL)
            self.Bind(wx.EVT_TEXT, self.tc_dsn_changed, self.tc_dsn)
            self.Bind(wx.EVT_TEXT, self.tc_jar_changed, self.tc_jar)
            self.Bind(wx.EVT_TEXT, self.tc_workdir_changed, self.tc_workdir)
            self.Bind(wx.EVT_COMMAND_SCROLL, self.sl_threads_scrolled, self.sl_threads)
            # end wxGlade

        def __set_properties(self):
            # begin wxGlade: MainFrame.__set_properties
            self.SetTitle("Forms 6 -> 10")
            self.SetSize((500, 400))
            self.rb_src.SetSelection(0)
            # end wxGlade
            self.options = globals()['options']
            self.cb_recurse.SetValue(self.options.recurse)
            self.rb_src.SetSelection({True: 0, False: 1}[self.options.src is None])
            self

        def __do_layout(self):
            # begin wxGlade: MainFrame.__do_layout
            sizer_1 = wx.BoxSizer(wx.VERTICAL)
            grid_sizer_1 = wx.FlexGridSizer(4, 2, 5, 10)
            sizer_3 = wx.BoxSizer(wx.VERTICAL)
            sizer_5 = wx.BoxSizer(wx.HORIZONTAL)
            sizer_2 = wx.StaticBoxSizer(self.sizer_2_staticbox, wx.HORIZONTAL)
            sizer_4 = wx.BoxSizer(wx.VERTICAL)
            sizer_3_copy = wx.BoxSizer(wx.HORIZONTAL)
            sizer_2.Add(self.rb_src, 0, wx.ADJUST_MINSIZE, 0)
            sizer_3_copy.Add(self.tc_src, 1, wx.ALIGN_CENTER_VERTICAL|wx.ADJUST_MINSIZE, 0)
            sizer_3_copy.Add(self.bu_src, 0, wx.ALIGN_CENTER_VERTICAL, 0)
            sizer_4.Add(sizer_3_copy, 1, wx.EXPAND, 0)
            sizer_4.Add(self.cb_recurse, 0, wx.ADJUST_MINSIZE, 0)
            sizer_2.Add(sizer_4, 1, 0, 0)
            sizer_3.Add(sizer_2, 1, wx.EXPAND, 0)
            sizer_5.Add(self.bu_ok, 2, wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            sizer_5.Add(self.bu_cancel, 1, wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            sizer_3.Add(sizer_5, 1, wx.EXPAND, 0)
            self.notebook_1_pane_1.SetAutoLayout(True)
            self.notebook_1_pane_1.SetSizer(sizer_3)
            sizer_3.Fit(self.notebook_1_pane_1)
            sizer_3.SetSizeHints(self.notebook_1_pane_1)
            grid_sizer_1.Add(self.la_dsn, 0, wx.ALIGN_RIGHT|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.tc_dsn, 0, wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.la_jar, 0, wx.ALIGN_RIGHT|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.tc_jar, 0, wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.la_workdir, 0, wx.ALIGN_RIGHT|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.tc_workdir, 0, wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.la_threads, 0, wx.ALIGN_RIGHT|wx.ADJUST_MINSIZE, 0)
            grid_sizer_1.Add(self.sl_threads, 1, wx.EXPAND, 0)
            self.nb_opciok_pane_2.SetAutoLayout(True)
            self.nb_opciok_pane_2.SetSizer(grid_sizer_1)
            grid_sizer_1.Fit(self.nb_opciok_pane_2)
            grid_sizer_1.SetSizeHints(self.nb_opciok_pane_2)
            grid_sizer_1.AddGrowableCol(1)
            self.nb_opciok.AddPage(self.notebook_1_pane_1, u"Konvertálás")
            self.nb_opciok.AddPage(self.nb_opciok_pane_2, u"Opciók")
            sizer_1.Add(self.nb_opciok, 0, wx.EXPAND, 0)
            sizer_1.Add(self.tc_log, 1, wx.TOP|wx.EXPAND|wx.ADJUST_MINSIZE, 0)
            self.SetAutoLayout(True)
            self.SetSizer(sizer_1)
            self.Layout()
            # end wxGlade

        def rb_src_changed(self, event): # wxGlade: MainFrame.<event_handler>
            print "Event handler `rb_src_changed' not implemented"
            event.Skip()

        def bu_src_pressed(self, event): # wxGlade: MainFrame.<event_handler>
            print "Event handler `bu_src_pressed' not implemented"
            event.Skip()

        def cb_recurse_checked(self, event): # wxGlade: MainFrame.<event_handler>
            self.options.recurse = (self.cb_recurse.GetValue() == wx.CHK_CHECKED)

        def tc_dsn_changed(self, event): # wxGlade: MainFrame.<event_handler>
            txt = self.tc_dsn.GetValue()
            tmp = txt.split('@')
            if len(tmp) == 2 and '/' in tmp[0]: self.options.dsn = txt
            else: self.tc_jar.SetValue(unicode(self.options.dsn))

        def tc_jar_changed(self, event): # wxGlade: MainFrame.<event_handler>
            txt = self.tc_jar.GetValue()
            if os.path.isfile(txt): self.options.jar = txt
            else: self.tc_jar.SetValue(unicode(self.options.jar))

        def tc_workdir_changed(self, event): # wxGlade: MainFrame.<event_handler>
            self.options.workdir = self.tc_workdir.GetText()

        def bu_ok_pressed(self, event): # wxGlade: MainFrame.<event_handler>
            print "Event handler `bu_ok_pressed' not implemented"
            event.Skip()

        def bu_cancel_pressed(self, event): # wxGlade: MainFrame.<event_handler>
            print "Event handler `bu_cancel_pressed' not implemented"
            event.Skip()

        def sl_threads_scrolled(self, event): # wxGlade: MainFrame.<event_handler>
            self.options.threads = self.sl_threads.GetValue()
            event.Skip()

# end of class MainFrame


class Files(object):
    def __init__(self, src, dest, work_dir,
                 recurse=False, ext='.fmb', out_ext='fmb'):
        self.work_dir = work_dir
        self.src = os.path.abspath(src)
        self.ext = ext
        self.out_ext = out_ext
        self.recurse = recurse
        if '-' == dest:
            self.dest = sys.stdout
        elif '.' in dest:
            self.dest = dest
        else:
            if not os.path.exists(dest):
                LOG.warn("create %r" % dest)
                os.makedirs(dest)
            self.dest = dest
        self.files = set()

    def add(self, obj):
        if os.path.isdir(obj):
            self.add_dir(obj)
        else:
            self.add_file(obj)

    def add_dir(self, adir):
        absdir = os.path.abspath(adir)
        minus = len(leql(absdir, (self.src.endswith(os.path.sep) and [self.src] or [self.src + os.path.sep])[0]))

        #print 'add_dir(%r)' % absdir
        for root, dirs, files in os.walk(absdir):
            #print 'root: %s' % root
            if not self.recurse and root != absdir:
                continue
            for fn in (os.path.join(root, fn) for fn in files
                       if fn.endswith(self.ext)):
                dn = os.path.dirname(fn[minus:]).strip(os.path.sep)
                self.add_file(fn, dn)

    def add_file(self, inp_fn, adir=None):
        assert sys.stdout != self.dest or len(self.files) == 0
        assert os.path.exists(inp_fn)
        inp_fn = os.path.abspath(inp_fn)
        if adir is None: adir = ''
        ori_fn = os.path.basename(inp_fn)
        work_fn = os.path.join(self.work_dir, adir, ori_fn)

        out_fn = (sys.stdout == self.dest and ['-']
            or [os.path.join(self.dest, adir, chext(ori_fn, self.out_ext))])[0]

        # for dn in map(os.path.dirname, (work_fn, out_fn)):
            # if not os.path.exists(dn): os.makedirs(dn)
        # shutil.copyfile(inp_fn, work_fn)
        #print (inp_fn, work_fn, out_fn)
        self.files.add((inp_fn, work_fn, out_fn))

    def get_files(self):
        return dict([tuple(x[1:]) for x in self.files])

    def iteritems(self):
        for inp_fn, work_fn, out_fn in self.files:
            # print inp_fn, work_fn, out_fn
            for dn in (os.path.dirname(x) for x in (work_fn, out_fn)
                       if x and x != '-' ):
                if not '.' in dn and not os.path.exists(dn):
                    os.makedirs(dn)
#      print work_fn
            shutil.copyfile(inp_fn, work_fn)
            yield (work_fn, out_fn)

    def cleanup(self):
        pass

    def __len__(self): return len(self.files)
    def __str__(self): return str(self.files)

def chext(fn, ext):
    tmp = fn.split('.')
    return '.'.join(tmp[:-1] + [ext])

def leql(a, b):
    n = min(len(a), len(b))
    for i in xrange(0, n):
        if not a[i] == b[i]: return a[i:]
    return a[:n]

from xml.dom.minidom import parse as parsexml
class FormsXmlProcessor(object):
    (c_cell_width, c_cell_height) = (12, 24)
    def __init__(self, fn):
        self.fn = fn
        self.dom = None
        self.module_name = None
        self.module_version = None
        self.visual_attributes = {
            'used': set(['NORMAL_ITEM', 'SELECT', 'NORMAL_PROMPT', 'NORMAL']),
            'exists': set([])}
        self.unknown_parents = set([])

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

import subprocess
def run_quietly(cmd, msg=None):
    t = time()
    LOG.debug('run_quietly(%r)', cmd)
    try:
        p = subprocess.Popen(cmd, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE)
    except:
        LOG.exception(u'cannot start program %s' % u' '.join(dec(x) for x in cmd))
        sys.exit(1)
    out, err = p.communicate()
    #tup = (ch_stdout, ch_stdin, ch_stderr) = popen2.popen3(cmdline, mode='t')
    #stdout = ch_stdout.read()
    #ch_stdout.close()
    #stderr = ch_stderr.read()
    #ch_stderr.close()
    #status = ch_stdin.close()
    if msg:
        print msg % (time()-t)
    # print 'STATUS', status
    # print 'STDOUT', stdout
    # print 'STDERR', stderr
    #return (status, stdout, stderr)
    return (p.returncode, out, err)

def process_file(cmd, fn, out_fn, **kwds):
    global exceptions, verbosity, unknown_parents#, c_java_d
    c_java_d = globals()['c_java_d']
    #print "process_file(%r, %r, %r, %r)" % (cmd, fn, out_fn, kwds)
    if not fn: 
        return

    # print c_java_d, id(c_java_d)
    if not c_java_d.has_key('arglist'):
        classpath = os.pathsep.join(c_java_d['cp'])
        params = ' '.join('-D%s=%s' % tup
                for tup in c_java_d.get('D', {}).iteritems())
        if c_java_d['jvm'].endswith('.bat'):
            c_java_d['arglist'] = [c_java_d['jvm'], params, "-cp", classpath,
                                   c_java_d['class']]
        else:
            c_java_d['arglist'] = [c_java_d['jvm'], params, "-cp", classpath, '-jar',
                                   c_java_d['jar']]
    arglist = c_java_d['arglist']
    if verbosity > 0:
        print "arglist:", arglist
    t = time()
    if 'conv' == cmd and JAVA_JUST_DUMP:
        try:
            tmp_fh, tmp_fn = tempfile.mkstemp(#dir=r'c:\temp',
                    prefix='tf-xml-', suffix='.xml')
            os.close(tmp_fh)
        except RuntimeWarning: 
            pass
        if verbosity > 1: 
            print 'TMP:', tmp_fn
        #cmdline = ' '.join(arglist + ['dump', fn, tmp_fn])
        cmdline = arglist + ['dump', fn, tmp_fn]
        if verbosity > 0: 
            print '1:', cmdline
        #(ch_stdin, ch_stdout, ch_stderr) = os.popen3(cmdline)
        tup = run_quietly(cmdline, (verbosity > 0 and ['dump: %.03fs'] or [None])[0])
        if tup and len(tup) > 2 and tup[2] is not None and len(tup[2]) > 0:
            exceptions.append(tup)
            print tup[-1]
            return
        if verbosity > 0: print tup[1]
        P = FormsXmlProcessor(tmp_fn)
        t2 = time()
        P.run()
        if verbosity > 0: print 'processing xml: %.03fs' % (time()-t)
        for x in P.unknown_parents: unknown_parents.add(x)
        P.write(EncodingWriter(tmp_fn))
        #cmdline = ' '.join(arglist + ['load', kwds['dsn'], tmp_fn, out_fn])
        cmdline = arglist + ['load', kwds['dsn'], tmp_fn, out_fn]
        if verbosity > 0: print '2:', cmdline
        #(ch_stdin, ch_stdout, ch_stderr) = os.popen3(cmdline)
        # print 'START %s' % cmdline
        tup = run_quietly(cmdline, (verbosity > 0 and ['load: %.03fs'] or [None])[0])
        # print 'RETURN:', tup
        if tup and len(tup) > 2 and tup[2] is not None and len(tup[2]) > 0:
            exceptions.append(tup)
            print tup[-1]
            return False
        if verbosity > 1: print tup[1]
        i = N = 3
        while i > 0:
            try:
                # print tmp_fn, os.path.exists(tmp_fn)
                if os.path.exists(tmp_fn): os.unlink(tmp_fn)
                i = 0
            except:
                # print i, (N-i)*1.5
                sleep((N-i)*1.5)
            i -= 1
    elif 'load' == cmd:
        #cmdline = ' '.join(arglist + ['load', kwds['dsn'], fn, out_fn])
        cmdline = arglist + ['load', kwds['dsn'], fn, out_fn]
        if verbosity > 0: print '2:', cmdline
        #(ch_stdin, ch_stdout, ch_stderr) = os.popen3(cmdline)
        # print 'START %s' % cmdline
        tup = run_quietly(cmdline)
        # print 'RETURN:', tup
        if tup and len(tup) > 2 and tup[2] is not None and len(tup[2]) > 0:
            exceptions.append(tup)
            print tup[-1]
            return False
        if verbosity > 1: print tup[1]
    else:
        arguments = arglist + [cmd]
        if 'conv' == cmd: arguments.append(kwds['dsn'])
        print 'out_fn=', out_fn, fn
        tmp_fn = None
        if out_fn == '-': # stdoutra kell
            try:
                tmp_fh, tmp_fn = tempfile.mkstemp(#dir=r'c:\temp',
                        prefix='tf-xml-', suffix='.xml')
                os.close(tmp_fh)
            except RuntimeWarning: pass
        cmdline = ' '.join(arguments + [fn, (tmp_fn and [tmp_fn] or [out_fn])[0]])
        print cmdline
        os.system(cmdline)
        if tmp_fn: # stdoutra kell
            fh = file(tmp_fn, 'rb')
            while 1:
                buf = fh.read(8192)
                if buf is None or len(buf) == 0: break
                sys.stdout.write(buf)
            fh.close()
    #
    print '%s %s: %.03fs' % (cmd, fn, time()-t)
    return True

def start_as_needed(act, pool, step):
    u'''A pool-ból kipótolja az act-ban lévő futók darabszámát step-re'''
    for th in [th for th in act if not th.isAlive()]: act.remove(th)
    kov = min(max(0, step-len(act)), len(pool))
    for th in pool[:kov]:
        print u'Starting %s' % th.getName()
        th.start()
    act.extend(pool[:kov])
    del pool[:kov]

options = None
exceptions = []
verbosity = 0
unknown_parents = set([])
def main(*args, **kwds):
    global exceptions, unknown_parents, options#, c_java_d
    options = globals()['options']
    c_java_d = globals()['c_java_d']

    parser = optparse.OptionParser(usage=u'%prog <conv|dump> [opts] <file(s)>',
        version='%prog v0.1')
    parser.add_option('-d', '--dsn', help=u'user/pass@db')
    parser.add_option('--prefix', help=u'output filename prefix')
    parser.add_option('--postfix', help=u'output filename prefix')
    parser.add_option('--dest', help=u'destination directory')
    parser.add_option('--force', help=u'force overwriting files',
        action='store_true')
    parser.add_option('-w', '--work-dir', help=u'working directory',
        dest='work_dir')
    parser.add_option('-s', '--src', help=u'source dir', dest='src')
    parser.add_option('-r', '--recurse', help=u'recurse to subdirs',
        dest='recurse', action='store_true')
    parser.add_option('-t', '--threads', help=u'maximum thread number',
        dest='threads', type='int')
    parser.add_option('-v', '--verbose', dest='verbose', type='int')
    parser.add_option('--jar', default=c_java_d['jar'])
    parser.add_option('-o', '--out', dest='out_fn', default=None)
    parser.set_defaults(prefix='conv-', postfix='', force=False, work_dir='_tf',
        recurse=False, threads=1, src=None, verbose=0)
    (options, args) = parser.parse_args(list(args))
    if not os.path.exists(options.work_dir): 
        os.mkdir(options.work_dir)
    cmd = None
    if len(args) <= 2:
        if not options.recurse or len(args) < 2:
            if GUI:
                app = wx.PySimpleApp()
                main = MainFrame(None)
                main.Show()
                app.MainLoop()
            else:
                parser.error(u'At least 2 args needed!')
        else: 
            cmd = args[1]
    else: 
        cmd = args[1]

    global verbosity
    verbosity = options.verbose
    c_java_d['jar'] = options.jar
    c_java_d['cp'].extend([options.jar, os.path.dirname(os.path.abspath(options.jar))])
    # globals()['c_java_d'] = c_java_d

    if not cmd in ('conv', 'dump', 'load'): 
        parser.error(u'Rule %s is not found!' % cmd)
    else:
        opts = dict(options.__dict__)
        fs = []
        for fn in args[2:]:
            if os.path.exists(fn): fs.append(fn)
            else: fs.extend(glob.glob(fn))
        if opts['dest'] is None:
            if 1 == len(fs) and 'dump' == cmd:
                if opts['out_fn']:
                    opts['dest'] = os.path.dirname(os.path.abspath(opts['out_fn']))
                else: opts['dest'] = '-'
            else: opts['dest'] = '.'

        F = Files(opts['src'], opts['dest'], opts['work_dir'],
            recurse=opts['recurse'],
            out_ext={'conv': 'fmb', 'dump': 'xml', 'load': 'fmb'}[cmd])
        #for fn in fs: shutil.copy(fn, opts['work_dir'])
        for fn in fs:
            F.add(fn)
        if opts['src']:
            F.add_dir(opts['src'])

        t = time()
        files = F#.get_files()
        #print files
        if JYTHON and not JAVA_JUST_DUMP:
            if 2 == JYTHON:
                T = TransForm(getattr(rule, cmd), files, opts)
                T.run()
            else:
                arglist = [cmd,]
                if 'conv' == cmd: arglist.append(opts['dsn'])
                for fn, out in files.iteritems():
                    #print arglist + [fn, out]
                    TransForm.main(arglist + [fn, out])
        else:
            if False:
                for fn, out in files.iteritems():
                    process_file(cmd, fn, out, dsn=opts['dsn'])
            else:
                step = opts['threads']
                print u'gathering files...'
                i = 0
                pool = []
                act = []
                for fn, out in files.iteritems():
                    i += 1
                    th = Thread(target=process_file, args=[cmd, fn, out],
                                             kwargs={'dsn': opts['dsn']},
                                             name='%s %s' % (cmd, fn))
                    pool.append(th)
                    if i % step == 0:
                        start_as_needed(act, pool, step)
                        print chr(8)*8 + '%04d' % i,
                #
                start_as_needed(act, pool, step)
                print u'%d files found.' % len(pool)
                while len(act) > 0 and len(pool) > 0:# and not exceptions:
                    start_as_needed(act, pool, step)
                    sleep(1)
                    print (len(act), len(pool))
                while pool:
                    pool.pop(0).join()
        t2 = time()-t
        print ('%d fmb processed in %.03fs (%.03f s/fmb avg) using at most %d threads concurrently'
                     % (len(files), t2, (float(t2)/max(1, len(files))), opts['threads']))
        if unknown_parents: 
            print 'UNKNOWN_PARENTS:\n', pformat(unknown_parents)
        else: 
            print 'nincs unknown parent.'

if __name__ == '__main__':
    logging.basicConfig(level=logging.DEBUG)
    main(*sys.argv)

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

package unosoft.forms;

import java.io.File;
import java.io.InputStream;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.OutputStream;
import java.io.IOException;
import java.net.InetSocketAddress;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;

import oracle.forms.jdapi.Jdapi;
import oracle.forms.jdapi.JdapiModule;
import oracle.forms.util.xmltools.Forms2XML;
import oracle.forms.util.xmltools.XML2Forms;

public class Serve {
    private InetSocketAddress addr = null;
    private HttpServer server = null;

    public Serve(InetSocketAddress addr) throws IOException {
        this.addr = addr;
        this.server = HttpServer.create(addr, 10);

        String formsPath = System.getProperty("forms.lib.path");
        System.err.println("forms.lib.path=" + formsPath);
		String conn = System.getProperty("forms.db.conn");
        System.err.println("forms.db.conn=" + conn);
		Jdapi.connectToDatabase(conn);

        server.createContext("/", new ConvertHandler(formsPath));
        server.setExecutor(null); // creates a default executor
    }

    public static void main(String[] args) throws Exception {
        String addrS = ":8000";
        if (args.length > 0) {
            addrS = args[0];
        }
        String portS = addrS;
        int i = portS.lastIndexOf(':');
        if (i >= 0) {
            addrS = addrS.substring(0, i);
            portS = portS.substring(i + 1);
        } else {
            addrS = "127.0.0.1";
        }
        if (addrS.length() == 0) {
            addrS = "127.0.0.1";
        }
        int port = 8000;
        try {
            port = Integer.parseInt(portS);
        } catch (NumberFormatException e) {
            System.err.println(e);
            System.exit(1);
        }
        (new Serve(new InetSocketAddress(addrS, port))).Start();
    }

    public static long Copy(OutputStream os, InputStream is) throws java.io.IOException {
        byte[] b = new byte[65536];
        long n = 0;
        for (int i = is.read(b); i >= 0; i = is.read(b)) {
			os.write(b, 0, i);
			n += i;
        }
        return n;
    }

    public void Start() {
        System.err.println("Start listening on " + this.addr);
        // http://www.dbadvice.be/oracle-forms-java-api-jdapi/
        Jdapi.setFailLibraryLoad(false);
        Jdapi.setFailSubclassLoad(false);
        this.server.start();
    }

    class ConvertHandler implements HttpHandler {
        String formsPath = null;

		public ConvertHandler(String formsPath ) {
			this.formsPath = formsPath;
			System.err.println("Initialize with empty.fmb ...");
			if( false ) {
				try {
					new Forms2XML(new File("empty.fmb"));
					new XML2Forms(null);
				} catch(Exception e) {
					System.err.println(e.toString());
				}
			}
			System.err.println("Initialized successfully.");
		}

        @Override
        public void handle(HttpExchange t) throws IOException {
			System.out.println("Got "+t.getRequestMethod()+" with ct="+t.getRequestHeaders().getFirst("Content-Type"));
			OutputStream os = t.getResponseBody();
			if( !t.getRequestMethod().equals("POST") ) {
				byte[] response = ("only POST is allowed! (got "+t.getRequestMethod()+")").getBytes();
				t.sendResponseHeaders(403, response.length);
				os.write(response);
				os.close();
				return;
			}
			String ext = ".fmb";
			boolean fromXML = false;
			if( t.getRequestHeaders().getFirst("Content-Type").equals("application/xml") ||
					t.getRequestHeaders().getFirst("Accept").equals("application/x-oracle-forms") ) {
				ext = ".fmb.xml";
				fromXML = true;
			}
			System.err.println("ext="+ext+" fromXML="+String.valueOf(fromXML));

			File src = File.createTempFile("forms2xml-", ext);
			File dst = null;
			try {
				FileOutputStream fos = new FileOutputStream(src);
				long n = Serve.Copy(fos, t.getRequestBody());
				fos.close();
				System.err.println("Read "+n+" bytes into "+src.getName()+".");

				if( !fromXML ) {
					// fmb -> XML
					oracle.xml.parser.v2.XMLDocument xml = null;
						xml = (new Forms2XML(src)).dumpModule();
					//dst = File.createTempFile("fmb2xml-", ".xml");
					if( dst == null ) {
						t.getResponseHeaders().set("Content-Type", "application/xml");
						t.sendResponseHeaders(200, 0);
						xml.print(os);
						return;
					}
					xml.print(new FileOutputStream(dst));
					t.getResponseHeaders().set("Content-Type", "application/xml");
					t.getResponseHeaders().set("Location", "file://"+dst.getAbsolutePath());
					t.sendResponseHeaders(201, -1);
					return;
				}

				// XML -> fmb
				JdapiModule fmb = (new
						XML2Forms(new java.net.URL("file://"+src.getAbsolutePath()))).createModule();
				dst = File.createTempFile("fmb2xml-", ".fmb");
				fmb.save(dst.getAbsolutePath());
				t.getResponseHeaders().set("Content-Type", "application/x-oracle-forms");
				t.getResponseHeaders().set("Location", "file://"+dst.getAbsolutePath());
				t.sendResponseHeaders(201, 0);
			} catch(Exception e) {
				t.getResponseHeaders().set("Content-Type", "text/plain");
				byte[] response = ("ERROR: "+e.toString()).getBytes();
				t.sendResponseHeaders(500, response.length);
				os.write(response);
			} finally {
				os.close();
				src.delete();
			}
        }
    }

}

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

import java.io.*;
import java.net.InetSocketAddress;
import java.util.Scanner;

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
        server.createContext("/", new ConvertHandler(formsPath));
        server.setExecutor(null); // creates a default executor
    }

	public static void Initialize(String conn) {
        //String formsPath = System.getProperty("forms.lib.path");
        //System.out.println("forms.lib.path=" + formsPath);
        System.err.println("DBG connecting to" + conn);
		Jdapi.connectToDatabase(conn);
		System.err.println("DBG Initialize with empty.fmb ...");
		try {
			new Forms2XML(new File("empty.fmb"));
			new XML2Forms(null);
		} catch(Exception e) {
			System.err.println("ERR "+e.toString());
		}
		System.err.println("DBG Initialized successfully.");
	}

	private static void swap() {
		PrintStream ps = System.err;
		System.setErr(System.out);
		System.setOut( ps);
	}

    public static void main(String[] args) throws Exception {
		Initialize(System.getProperty("forms.db.conn"));

        String addrS = ":8000";
        if (args.length > 0) {
            addrS = args[0];
        }
		if (addrS.equals("-")) {
			String path = System.getProperty("user.dir");
			System.err.println("Waiting [<src> <dst>] pairs on stdin...");
			StreamTokenizer st = new StreamTokenizer(System.in);
			st.eolIsSignificant(true);
			String[] toks = new String[2];
			int pos = 0;
			while(true) {
				int tt;
				tt = st.nextToken();
				System.err.println("TOK "+String.valueOf(tt) + " = "+st.sval);
				if( tt == StreamTokenizer.TT_EOF) {
					break;
				}
				if( tt != StreamTokenizer.TT_EOL ) {
					toks[pos] = st.sval;
					pos++;
					continue;
				} // EOL
				System.err.println("DBG pos="+String.valueOf(pos));
				int act = pos;
				pos = 0;
				if( act != 2 ) {
					System.out.println("ERR need two args (got) "+String.valueOf(act));
					continue;
				}
				File src = toks[0].startsWith("/") ? new File(toks[0]) : new File( path, toks[0]);
				if( !src.exists() ) {
					System.out.println("ERR file not exist: "+src.getName());
					continue;
				}
				File dst = toks[1].startsWith("/") ? new File(toks[1]) : new File( path, toks[1]);
				if( !dst.exists() ) {
					System.out.println("ERR file not exist: "+dst.getName());
					continue;
				}

				if( src.getName().endsWith(".fmb") ) {
					// fmb -> XML
					oracle.xml.parser.v2.XMLDocument xml = null;
					System.err.println("DBG load "+src.getAbsolutePath());
					try {
						swap();
						try {
							xml = (new Forms2XML(src)).dumpModule();
							xml.print(new FileOutputStream(dst));
						} finally {
							swap();
						}
					} catch(Exception e) {
						System.out.println("ERR "+e.toString());
						continue;
					}
					System.out.println("OK+ dumped "+src.getName()+" to "+dst);
					continue;
				}

				// XML -> fmb
				try {
					swap();
					try {
						JdapiModule fmb = (new
								XML2Forms(new java.net.URL("file://"+src))).createModule();
						fmb.save(dst.getName());
					} finally {
						swap();
					}
				} catch(Exception e) {
					System.out.println("ERR "+e.toString());
					continue;
				}
				System.out.println("OK+ written to "+dst);
			}
			return;
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
        System.out.println("Start listening on " + this.addr);
        // http://www.dbadvice.be/oracle-forms-java-api-jdapi/
        Jdapi.setFailLibraryLoad(false);
        Jdapi.setFailSubclassLoad(false);
        this.server.start();
    }

    class ConvertHandler implements HttpHandler {
        String formsPath = null;

		public ConvertHandler(String formsPath ) {
			this.formsPath = formsPath;
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
			System.out.println("ext="+ext+" fromXML="+String.valueOf(fromXML));

			File src = File.createTempFile("forms2xml-", ext);
			try {
				FileOutputStream fos = new FileOutputStream(src);
				long n = Serve.Copy(fos, t.getRequestBody());
				fos.close();
				System.out.println("Read "+n+" bytes into "+src.getName()+".");

				if( !fromXML ) {
					// fmb -> XML
					oracle.xml.parser.v2.XMLDocument xml = null;
					try {
						xml = (new Forms2XML(src)).dumpModule();
					} catch(Exception e) {
						t.getResponseHeaders().set("Content-Type", "text/plain");
						byte[] response = ("ERROR: "+e.toString()).getBytes();
						t.sendResponseHeaders(500, response.length);
						os.write(response);
						return;
					}
					t.getResponseHeaders().set("Content-Type", "application/xml");
					t.sendResponseHeaders(200, 0);
					xml.print(os);
					return;
				}

				// XML -> fmb
				try {
					JdapiModule fmb = (new
							XML2Forms(new java.net.URL("file://"+src.getAbsolutePath()))).createModule();
					File dst = File.createTempFile("fmb2xml-", ".fmb");
					t.getResponseHeaders().set("Content-Type", "application/x-oracle-forms");
					t.sendResponseHeaders(200, 0);
					try {
						fmb.save(dst.getAbsolutePath());
						n = Serve.Copy(os, new FileInputStream(dst));
						System.out.println("Written "+n+" bytes from "+dst.getName());
					} finally {
						dst.delete();
					}
				} catch( Exception e ) {
					byte[] response = ("ERROR: "+e.toString()).getBytes();
					t.sendResponseHeaders(500, response.length);
					os.write(response);
					return;
				}

			} finally {
				os.close();
				src.delete();
			}
        }
    }

}

package unosoft.forms;

import java.io.IOException;
import java.io.OutputStream;
import java.io.File;
import java.io.InputStream;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.net.InetSocketAddress;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;

import oracle.forms.jdapi.Jdapi;
import oracle.forms.jdapi.JdapiModule;
import oracle.forms.util.xmltools.Forms2XML;

public class Serve {
	private InetSocketAddress addr = null;
	private HttpServer server = null;

	public Serve(InetSocketAddress addr) throws IOException {
		this.addr = addr;
        this.server = HttpServer.create(addr, 10);

		String formsPath = System.getenv().get("BRUNO_HOME");
		if( formsPath.isEmpty() ) {
			formsPath = System.getenv().get("ABLAK_HOME");
			Jdapi.connectToDatabase(System.getenv().get("ABLAK_ID"));
		} else {
			Jdapi.connectToDatabase(System.getenv().get("BRUNO_ID"));
		}
		formsPath += "/../lib";
		System.out.println("formsPath="+formsPath);

        server.createContext("/", new ConvertHandler(formsPath));
        server.setExecutor(null); // creates a default executor
	}

    public static void main(String[] args) throws Exception {
		String addrS = ":8000";
		if( args.length > 0) {
			addrS = args[0];
		}
		String portS = addrS;
		int i = portS.lastIndexOf(':');
		if( i >= 0 ) {
			addrS = addrS.substring(0, i);
			portS = portS.substring(i+1);
		} else {
			addrS = "127.0.0.1";
		}
		if( addrS.length() == 0 ) {
			addrS = "127.0.0.1";
		}
		int port = 8000;
		try {
			port = Integer.parseInt(portS);
		} catch(NumberFormatException e) {
			System.err.println(e);
			System.exit(1);
		}
		(new Serve(new InetSocketAddress(addrS, port))).Start();
	}

	public void Start() {
		System.out.println("Start listening on "+this.addr);
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

			File src = File.createTempFile("forms2xml-", ext);
			try {
				long n = Serve.Copy(new FileOutputStream(src), t.getRequestBody());
				System.out.println("Read "+n+" bytes into "+src.toString()+".");

				if( !fromXML ) {
					// fmb -> XML
					oracle.xml.parser.v2.XMLDocument xml = null;
					try {
						xml = (new Forms2XML(src)).dumpModule();
					} catch(Exception e) {
						t.getResponseHeaders().set("Content-Type", "text/plain");
						byte[] response = ("ERROR with "+src+": "+e.toString()).getBytes();
						System.out.write(response);
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
							oracle.forms.util.xmltools.XML2Forms(new java.net.URL("file://"+src.getAbsolutePath()))).createModule();
					File dst = File.createTempFile("fmb2xml-", ".fmb");
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

	public static long Copy(OutputStream os, InputStream is) throws java.io.IOException {
		byte[] b = new byte[65536];
		long n = 0;
		for( int i = is.read(b); i > 0; i = is.read(b) ) {
			os.write(b, 0, i);
			n += i;
		}
		return n;
	}

}

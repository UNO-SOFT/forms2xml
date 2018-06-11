#!/bin/sh
set -e
cd "$(cd "$(dirname "$0")"; pwd)"
export ORACLE_BASE=/oracle/mw11gR1
export ORACLE_HOME=$ORACLE_BASE/fr11gR2
# /oracle/mw11gR1/fr11gR2/lib/libfrmjapi.so.0
export LD_LIBRARY_PATH=$ORACLE_HOME/bin:$ORACLE_HOME/lib:$LD_LIBRARY_PATH
export TERM=xterm

CLASSPATH=$CLASSPATH:classes:$ORACLE_HOME/jlib/frmjdapi.jar:$ORACLE_HOME/jlib/frmxmltools.jar:$ORACLE_HOME/lib/xmlparserv2.jar
if ! [ "${NOCOMPILE:-0}" -eq 1 ]; then
	mkdir -p classes
	javac -cp $CLASSPATH -d classes src/unosoft/forms/Serve.java
fi
export -p

if [ -z "$DISPLAY" ]; then
	echo
	echo '*********************************'
	echo "* DISPLAY is empty, won't work! *" >&2
	echo '*********************************'
	echo
fi
set -x
exec java -cp "$CLASSPATH" -Djava.library.path=$ORACLE_HOME/lib \
	"-Dforms.lib.path=$FORMS_LIB" \
	"-Dforms.db.conn=$DB_CONN" \
	unosoft.forms.Serve "$@"

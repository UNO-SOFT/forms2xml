@echo off
set ORACLE_BASE=C:\oracle\mw11gR1
set ORACLE_HOME=%ORACLE_BASE%\fr11gR2
set PATH=%ORACLE_HOME%\BIN;%ORACLE_HOME%\lib;%PATH%
set JAVA_HOME=%ORACLE_BASE%\jdk
set CLASSPATH=classes;%ORACLE_HOME%\jlib\frmjdapi.jar;%ORACLE_HOME%\jlib\frmxmltools.jar;%ORACLE_HOME%\lib\xmlparserv2.jar
mkdir -p classes

@echo on
%JAVA_HOME%\bin\javac -cp %CLASSPATH% -d classes src/unosoft/forms/Serve.java
@if %errorlevel% neq 0 exit /b %errorlevel%
set
%JAVA_HOME%\bin\java -cp %CLASSPATH% ^
    -Djava.library.path=%ORACLE_HOME%\BIN ^
    -Dforms.lib.path=%FORMS_LIB% ^
    -Dforms.db.conn=%DB_CONN% ^
    unosoft.forms.Serve %*


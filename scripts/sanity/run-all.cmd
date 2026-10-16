@echo off
rem Copyright (c) 2026 McSparrow. All rights reserved.
rem McHarbor is licensed under the McHarbor License. See LICENSE for details.
rem
rem Runs the McHarbor sanity suite from cmd.exe / double-click.
rem Any arguments are passed through to run-all.ps1, e.g.:
rem   run-all.cmd -Check security,permissions -SkipNetwork
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0run-all.ps1" %*
exit /b %ERRORLEVEL%

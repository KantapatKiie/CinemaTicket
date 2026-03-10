@echo off
setlocal

cd /d %~dp0\..
echo [run-all] starting mongo, redis, backend, frontend
docker compose up --build

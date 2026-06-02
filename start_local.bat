@echo off
title New-API Gateway Startup Script
echo ===================================================
echo   New-API Gateway - Chuamgwei Local Startup
echo ===================================================

set PORT=3000

set "SQL_DSN=root:your_mysql_password@tcp(127.0.0.1:13306)/chuamgwei_gateway?charset=utf8mb4&parseTime=True&loc=Local"

set "REDIS_CONN_STRING=redis://:your_redis_password@127.0.0.1:6379/0"

set REGISTER_DISABLED=true

set SESSION_SECRET=secret_chuamgwei_session_gateway_newapi_982

set CHANNEL_UPDATE_FREQUENCY=1
set CHANNEL_TEST_FREQUENCY=5

echo [*] Target DB: 47.94.16.23:13306/chuamgwei_gateway
echo [*] Target Redis: 47.94.16.23:26739
echo [*] Gateway Port: 3000
echo [*] Register Disabled: TRUE
echo ===================================================
echo Starting Go gateway service (New-API)...
echo ===================================================

go version >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Local Go environment not found. Please install Go or deploy it via Docker container.
    pause
    exit /b
)

go run main.go

pause

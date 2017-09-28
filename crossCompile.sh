#!/bin/bash

rm -rf compile
mkdir compile
env GOOS=linux GORACH=amd64 go build -o compile/bitsync_linux_amd64 main.go
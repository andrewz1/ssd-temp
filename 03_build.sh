#!/usr/bin/env bash

[ -r .env ] && source .env
exec_name=`basename "$(pwd)"`
rm -f "$exec_name"
go build || exit 1
strip "$exec_name"

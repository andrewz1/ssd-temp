#!/usr/bin/env bash

[ -r .env ] && source .env
go fmt ./...
git add -A
git commit -am cleanup
for r in `git remote` ; do
	git push $r
	git push --tags $r
done

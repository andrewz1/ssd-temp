#!/usr/bin/env bash

go fmt ./...
git add -A
git commit -am cleanup
for r in `git remote` ; do
	git push $r
	git push --tags $r
done

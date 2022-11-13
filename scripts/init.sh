#!/bin/bash

set -e

mkdir kuberik
cd kuberik
kubebuilder init --domain kuberik.io --repo github.com/kuberik/kuberik
make manifests

cd ..
shopt -s dotglob
mv kuberik/* .
rmdir kuberik

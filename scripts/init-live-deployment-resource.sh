#!/bin/sh

set -e

cd $(git rev-parse --show-toplevel)
yes | kubebuilder create api --version v1alpha1 --kind LiveDeployment
make manifests

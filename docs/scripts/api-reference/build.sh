#!/bin/sh

set -e

GENREF_VERSION=2c8f7e809fd890f52404f08803836ec870080af3
API_REF_SCRIPT_DIR=$(dirname $(realpath "$0"))
GENREF_REPO_DIR=${API_REF_SCRIPT_DIR}/genref
GENREF=${GENREF_REPO_DIR}/genref/genref
BUILD_TEMP_OUT=${API_REF_SCRIPT_DIR}/build

install_genref () {
    mkdir -p ${GENREF_REPO_DIR}
    cd ${GENREF_REPO_DIR}
    git init

    # Create origin
    git remote remove origin || true 2> /dev/null
    git remote add origin https://github.com/kubernetes-sigs/reference-docs.git

    # Configure sparse checkout
    git config core.sparsecheckout 1
    git config extensions.partialClone origin
    rm -f .git/info/sparse-checkout 2> /dev/null
    echo genref > .git/info/sparse-checkout

    # Fetch commit
    git fetch --depth 1 --filter=blob:none origin ${GENREF_VERSION}

    # Checkout files
    git checkout ${GENREF_VERSION}

    # Build the binary
    cd genref
    make genref
}

build () {
    cd $API_REF_SCRIPT_DIR
    pkill -f ${GENREF} || true
    ${GENREF} -o ${BUILD_TEMP_OUT}
}

if ! build; then
    install_genref
    build
fi

SITE_DIR=${API_REF_SCRIPT_DIR}/../../site
API_REFERENCE_DIR=${SITE_DIR}/api-reference
mkdir -p ${API_REFERENCE_DIR}
cat ${API_REF_SCRIPT_DIR}/index-header.md ${BUILD_TEMP_OUT}/* > ${API_REFERENCE_DIR}/index.md

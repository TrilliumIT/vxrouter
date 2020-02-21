#!/bin/bash
set -e

check_prerequisites() {
	[[ "$GOPATH" == "" ]] && \
		errors=(${errors[@]} "GOPATH env missing")
	
	return 0
}

check_versions() {
	VERS="${LATEST_RELEASE}\n${MAIN_VER}"
	DKR_TAG="master"

	# For tagged commits
	if [ "$(git describe --tags)" = "$(git describe --tags --abbrev=0)" ] ; then
		if [ $(printf ${VERS} | uniq | wc -l) -gt 1 ] ; then
			echo "This is a release, all versions should match"
			return 1
		fi
		DKR_TAG="latest"
	else
		if [ $(printf ${VERS} | uniq | wc -l) -eq 1 ] ; then
			echo "Please increment the version in main.go"
			return 1
		fi
		if [ "$(printf ${VERS} | sort -V | tail -n 1)" != "${MAIN_VER}" ] ; then
			echo "Please increment the version in main.go"
			return 1
		fi
	fi
}

LATEST_RELEASE=$(git describe --tags --abbrev=0 | sed "s/^v//g")
MAIN_VER=$(grep "\t*Version *= " const.go | sed 's/\t*Version *= //g' | sed 's/"//g')

check_prerequisites || exit 1
check_versions || exit 1

echo "Building..."
mkdir bin 2>/dev/null || true
go build -o bin/vxrnet ./docker/vxrnet

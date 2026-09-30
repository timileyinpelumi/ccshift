#!/usr/bin/env bash
# Adds .deb packages to the apt repository kept on the gh-pages branch, rebuilds its indexes and
# signs them. The signing key must already be in the gpg keyring.
#   usage: scripts/apt-publish.sh <directory with .deb files> <checkout of gh-pages> <key id>
set -euo pipefail
debs=$1
site=$(cd "$2" && pwd)
key=$3
repo=$site/apt

mkdir -p "$repo/pool/main/c/ccshift"
# Release assets have no version in their names; pool files need one so versions sit side by side.
for deb in "$debs"/*.deb; do
	name="ccshift_$(dpkg-deb --field "$deb" Version)_$(dpkg-deb --field "$deb" Architecture).deb"
	cp "$deb" "$repo/pool/main/c/ccshift/$name"
done
cd "$repo"
for arch in amd64 arm64; do
	dir=dists/stable/main/binary-$arch
	mkdir -p "$dir"
	apt-ftparchive --arch "$arch" packages pool >"$dir/Packages"
	gzip -9 -kf "$dir/Packages"
done
apt-ftparchive \
	-o APT::FTPArchive::Release::Origin=ccshift \
	-o APT::FTPArchive::Release::Label=ccshift \
	-o APT::FTPArchive::Release::Suite=stable \
	-o APT::FTPArchive::Release::Codename=stable \
	-o APT::FTPArchive::Release::Architectures="amd64 arm64" \
	-o APT::FTPArchive::Release::Components=main \
	-o APT::FTPArchive::Release::Description="ccshift packages" \
	release dists/stable >dists/stable/Release
gpg --batch --yes --default-key "$key" --clearsign -o dists/stable/InRelease dists/stable/Release
gpg --batch --yes --default-key "$key" -abs -o dists/stable/Release.gpg dists/stable/Release
gpg --batch --yes --export "$key" >"$repo/ccshift.gpg"

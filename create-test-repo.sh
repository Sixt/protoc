#!/bin/sh

set -e

# Create bare repo
mkdir testrepo
cd testrepo
git init --bare
cd ..

# Clone and create initial commit
git clone testrepo testclone
cd testclone
cat > test.proto << EOF
syntax = "proto3";
message Foo {
	string bar = 1;
}
EOF
git add test.proto
git commit -m "add test.proto v1.0.0"
git tag v1.0.0

# Modify proto file, commit and tag
cat > test.proto << EOF
syntax = "proto3";
message Foo {
	string bar = 1;
	string baz = 2;
}
EOF
git add test.proto
git commit -m "add test.proto v2.0.0"
git tag -a -m 'version 2' v2.0.0
git push
git push --tags

# Pack bare repo
cd ..
rm -rf testclone
zip -r testrepo testrepo
rm -rf testrepo


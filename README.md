# protoc

This is a small, simple and backwards-compatible [protoc][protoc] wrapper.

* [Features](#features)
* [How it works](#how-it-works)
* [How to use it in Go](#how-to-use-it-in-go)
* [How to use it in Java](#how-to-use-it-in-java)

## Features

It is go-gettable. One can do `go get github.com/sixt/protoc/v3` to install protoc. This enables managing protoc version inside `go.mod` when using the [recommended][tools-go] `tools.go` approach.

It supports proto files hosted in Git repositories. One can specify a local path to the proto file, or a remote URL, e.g. `github.com/myorg/myrepo/foo.proto`.

One can also specify a commit hash or a git tag to use a specific revision of the proto file: `protoc example.org/file.proto@v1.2.3`. If no revision is specified - the latest HEAD revision is used.

Cache is synced up with the remote only when the requested tag or revision is not found. This means if you use a remote proto file without specifying a particular commit hash or git tag - the initially fetched revision will be used. A special revision name `latest` can be used to invalidate the cache. In this case, the old cached repository is removed and is cloned once again from scratch.

Wrapper binaries are also published to Maven repo, so that they could be used in Java build process as well.

## How it works

First of all, wrapper extracts the real `protoc` binary into the user's cache directory. Default cache directory on Linux is ~/.cache/protoc (unless `$XDG_CACHE_HOME` is provided). Default cache directory on macOS is ~/Library/Caches. Protoc binary is extracted only once, if there is an existing binary in the cache with the matching checksum - it will be used instead.

Then, wrapper parses all command line flags. If an argument looks like a path to the proto file - wrapper checks whether the path exists on the local machine. If not - then it's likely to be a remote proto file URL.

In this case, wrapper clones the remote Git repo, fetches the requested revision, and replaces the remote URL with a path to the local file in the cache. Similarly, if remote Git repo is provided as an include path using `-I` or `--proto_path` flag - it gets substituted with a locally cached path.

Default git implementation uses command line git tool. Wrapper also supports build constraint `gogit` that uses [go-git][go-git] library for git fetch and checkout. It is known to be slower than command-line git, but might be helpful if command-line git tool is unavailable.

Wrapper supports authentication via `$HOME/.gitconfig`. It always uses https scheme for fetching, but one can specify `insteadOf` rule to use ssh for particular URLs. Go-git build variant is a bit different and supports authentication via `$HOME/.netrc` username/password, or SSH (using `$HOME/.ssh/id_rsa` keys).

## How to use it in Go

It is recommented to use `go:generate` statements to generate protobuf code from the proto files.

Create `tools.go` file with a build constraint `// +build tools`.

Add blank imports for protoc and the plugins you need.

Add go generate statements explicitly calling protoc with required command line options.

Here's a sample tools.go:

```go
//+build tools
package example

//go:generate go install github.com/sixt/protoc/v3
//go:generate go install github.com/golang/protobuf/protoc-gen-go
//go:generate go install github.com/micro/protoc-gen-micro

//go:generate protoc --go_out=. foo.proto

import (
	_ "github.com/sixt/protoc/v3"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/micro/protoc-gen-micro"
)
```

Running `go mod tidy` will add tools dependencies to your `go.mod` and `go.sum`.

Running `go generate -tags tools` will install all necessary tools and generate protobuf code.

You might want to set `$GOBIN` to be somewhere local to the project, so that if you are using different versions of the tools - they would not overwrite themselves every time you build another project.

## How to use it in Java

If using Gradle as a build system, you will need to create a custom task that will be generating proto classes for you and adds the generated code to the source sets.

```gradle
repositories {
  mavenCentral()
  maven {
    url "https://maven.pkg.github.com/sixt/protoc"
  }
}

apply plugin: 'java'

dependencies {
  compile 'com.google.protobuf:protobuf-java:+'
}

// Create separate configuration for protoc artifacts
configurations { protoc }
dependencies {
  protoc group: 'com.sixt.protobuf', name: 'protoc', version: '3.10.0', ext: 'exe',
  classifier: [
    'Windows:amd64': 'windows-x86_64',
    'Windows:i386': 'windows-x86_32',
    'Linux:amd64': 'linux-x86_64',
    'Linux:i386': 'linux-x86_32',
    'Mac OS X:amd64': 'osx-x86_64',
    'Mac OS X:x86_64': 'osx-x86_64',
  ]["${System.getProperty("os.name").toLowerCase().contains("windows") ? "Windows" : System.getProperty('os.name')}:${System.getProperty('os.arch')}"]
}

// Custom task that generates protobuf code in the build directory and adds it to the classpath
task proto(type: Exec) {
  dependsOn configurations.protoc
  sourceSets.main.java.srcDir file("${buildDir}/proto")
  def file = configurations.protoc.singleFile;
  if (!file.canExecute() && !file.setExecutable(true)) {
    throw new GradleException("Cannot set ${file} as executable")
  }
  doFirst { mkdir "${buildDir}/proto" }
  // Actual command line to protoc with all required options
  commandLine file.getAbsolutePath(),
    "-I.",
    "--java_out=${buildDir}/proto",
    "src/main/proto/foo.proto"
}
// Let compileJava always run protoc first
compileJava.dependsOn(proto)
```

## Versioning

To avoid confusion between the original Google's protoc compiler and this wrapper it was decided to use the following versioning: major, minor and patch versions of the wrapper MUST correspond to the one of the underlying protoc compiler. However, if a bugfix is required in a wrapper while keeping the same version of the original protoc compiler - a patch version is increased with a pre-released identifier appended.

Here's an example. Protoc 1.2.3 is released and wrapped with a version v1.2.3 as well. Then, a bug in a wrapper gets fixed and wrapper v1.2.4-1 is released.  Another bug is fixed and v1.2.4-2 is released. It takes higher priority than v1.2.3, but would be lower than v1.2.4. So once protoc 1.2.4 is released and wrapped under the version v1.2.4 - that will include our fixes and take the priority. This versioning approach should work with all systems that suppport
semver.

[protoc]: https://github.com/protocolbuffers/protobuf/tree/master/src
[tools-go]: https://golang.org/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module]
[go-git]: https://github.com/src-d/go-git


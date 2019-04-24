# protoc

This is a small and simple [protoc][protoc] wrapper.

- [protoc](#protoc)
  * [Features:](#markdown-header-features-)
  * [How it works](#markdown-header-how-it-works)
  * [How to use it in Go](#markdown-header-how-to-use-it-in-go)
  * [How to use it in Java](#markdown-header-how-to-use-it-in-java)

## Features:

It is go-gettable. One can do `go get bitbucket.org/sixtgoorange/protoc` to
install protoc. This enables managing protoc version inside `go.mod` when using
the [recommended][tools-go] `tools.go` approach.

It supports proto files hosted in Git repositories. One can specify a local
path to the proto file, or a remote URL, e.g. bitbucket.org/foo/bar/baz.proto.

One can also specify a commit hash or a git tag to use a specific revision of
the proto file: `protoc example.org/file.proto@v1.2.3`. If no revision is
specified - the latest HEAD revision is used.

Additionally, if no revision is specified then on every run "git pull" will be
executed trying to fetch the latest revision. Errors are ignored if it fails,
making it safe to use offline. A special reserved revision "latest" can be used
to force removal of the repository and cloning the most recent version. This
should be used to invalidate git cache for a particular revision.

Both cases (no revision or "latest" revision) are slower than using particular
tags or commit hashes, where local git cache is reused without network access.

Wrapper binaries are also published to Nexus, so that they could be used in
Java build process as well.

## How it works

First of all, wrapper extracts the real `protoc` binary into the user's cache
directory. Default cache directory on Linux is ~/.cache/protoc (unless
`$XDG_CACHE_HOME` is provided). Default cache directory on macOS is
~/Library/Caches. Protoc binary is extracted only once, if there is an existing
binary in the cache with the matching checksum - it will be used instead.

Then, wrapper parses all command line flags. If an argument looks like a path
to the proto file - wrapper checks whether the path exists on the local
machine. If not - then it's likely to be a remote proto file URL.

In this case, wrapper clones the remote Git repo, fetches the requested
revision, and replaces the remote URL with a path to the local file in the
cache.

For cloning/pulling repositories wrapper uses [go-git][go-git] library. It
supports authentication via `$HOME/.netrc` username/password, or SSH (using
`$HOME/.ssh/id_rsa` keys).

## How to use it in Go

It is recommented to use `go:generate` statements to generate protobuf code
from the proto files.

Create `tools.go` file with a build constraint `// +build tools`.

Add blank imports for protoc and the plugins you need.

Add go generate statements explicitly calling protoc with required command line
options.

Here's a sample tools.go:

```
//+build tools
package example

//go:generate go install bitbucket.org/sixtgoorange/protoc
//go:generate go install github.com/golang/protobuf/protoc-gen-go
//go:generate go install github.com/micro/protoc-gen-micro

//go:generate protoc --go_out=. --micro_out=. foo.proto

import (
	_ "bitbucket.org/sixtgoorange/protoc"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/micro/protoc-gen-micro"
)
```

Running `go mod tidy` will add tools dependencies to your `go.mod` and `go.sum`.

Running `go generate -tags tools` will install all tools and generate protobuf
code.

You might want to set `$GOBIN` to be somewhere local to the project, so that if
you are using different versions of the tools - they would not overwrite
themselves every time you build another project.

## How to use it in Java

It is recommented to use Gradle as a build system. Then you will need to create
a custom task that will be generating proto classes for you and will add
generate code to the source sets.

```
repositories {
  mavenCentral()
  maven {
    url 'https://nexus.goorange.sixt.com/nexus/content/repositories/releases/'
  }
}

apply plugin: 'java'

dependencies {
  compile 'com.google.protobuf:protobuf-java:+'
}

// Create separate configuration for protoc artifacts
configurations { protoc }
dependencies {
  protoc group: 'com.sixt.protobuf', name: 'protoc', version: '0.0.0-dev', ext: 'exe',
    classifier: [
      'Linux:amd64': 'linux-x86_64',
      'Linux:i386': 'linux-x86_32',
      'Mac OS X:amd64': 'osx-x86_64',
      'Mac OS X:x86_64': 'osx-x86_64',
    ]["${System.getProperty('os.name')}:${System.getProperty('os.arch')}"]
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
    "src/main/proto/foo.proto" // or "bitbucket.org/foo/bar/baz.proto"
}
// Let compileJava always run protoc first
compileJava.dependsOn(proto)
```

[protoc]: https://github.com/protocolbuffers/protobuf/tree/master/src
[tools-go]: https://golang.org/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module]
[go-git]: https://github.com/src-d/go-git

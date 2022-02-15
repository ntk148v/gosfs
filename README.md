<div align="center">
	<h1>Golang SFS (Golang simple file server)</h1>
	<blockquote align="center">Github repository template for Go to quickly create a new repository.</blockquote>
	<p>
		<a href="https://github.com/ntk148v/gosfs/blob/master/LICENSE">
			<img alt="GitHub license" src="https://img.shields.io/github/license/ntk148v/gosfs?style=for-the-badge">
		</a>
		<a href="https://github.com/ntk148v/gosfs/stargazers">
			<img alt="GitHub stars" src="https://img.shields.io/github/stars/ntk148v/gosfs?style=for-the-badge">
		</a>
		<br>
<!--		<a href="https://github.com/ntk148v/gosfs/actions">
			<img alt="Windows Build Status" src="https://img.shields.io/github/workflow/status/ntk148v/gosfs/Windows%20Build?style=flat-square&logo=github&label=Windows">
		</a>
		<a href="https://github.com/ntk148v/gosfs/actions">
			<img alt="GNU/Linux Build Status" src="https://img.shields.io/github/workflow/status/ntk148v/gosfs/Linux%20Build?style=flat-square&logo=github&label=GNU/Linux">
		</a>
		<a href="https://github.com/ntk148v/gosfs/actions">
			<img alt="MacOS Build Status" src="https://img.shields.io/github/workflow/status/ntk148v/gosfs/MacOS%20Build?style=flat-square&logo=github&label=MacOS">
		</a>
		<br>-->
	</p><br>
</div>

Simple HTTP server written in pure Golang to serve and upload files.

## Getting started

```bash
$ go run main.go --help
Usage of /tmp/go-build2896055445/b001/exe/main:
  -bind-addr string
        IP address to bind (default "0.0.0.0")
  -max-size int
        max size of uploaded file (byte) (default 5242880)
  -port int
        port number to listen on (default 2690)
  -root-dir string
        root directory (default "/tmp/gosfs")
```

### TODO

- Multiple upload

## Screenshots

![](screenshots/screen1.png)

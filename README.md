# httpbin

Testing an HTTP Library can become difficult sometimes.
[httpbin](https://github.com/six-ddc/httpbin) is fantastic for testing HTTP requests, and easy to control the response.

## Installation

```bash
$ go get https://github.com/six-ddc/httpbin
```

## Usage

```bash
$ httpbin <adress> <script-file>
$ httpbin <adress> -c <commands>
```

## Examples

```bash
httpbin :8080/hello -c 'httpbin add body world'

curl http://127.0.0.1:8080/hello
## world
```

```bash
httpbin :8080/ip -c 'ip=$(httpbin get ip); httpbin add body $ip'

curl http://127.0.0.1:8080/ip
## 127.0.0.1
```

```bash
httpbin :8080/logo.gif -c 'curl "http://www.sinaimg.cn/blog/developer/wiki/LOGO_64x64.png" | httpbin add body'

curl -O http://127.0.0.1:8080/logo.gif
## [logo.gif]
```

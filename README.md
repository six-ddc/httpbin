# httpbin

Testing an HTTP Library can become difficult sometimes.
[httpbin](https://github.com/six-ddc/httpbin) is fantastic for testing HTTP requests, and easy to control the response.

## Installation

```bash
$ go get https://github.com/six-ddc/httpbin
```

## Usage

```bash
# listen on the address, and execute 'bash -c <commands>' for each accepted HTTP request
$ httpbin <adress> -c <commands>

# listen on the address, and execute 'bash <script-file>' for each accepted HTTP request
$ httpbin <adress> <script-file>
```

## Examples

```bash
$ httpbin :8080/hello -c 'httpbin add body world'

$ curl http://127.0.0.1:8080/hello
## world
```

```bash
$ httpbin :8080/ip -c 'ip=$(httpbin get ip); httpbin add body $ip'

$ curl http://127.0.0.1:8080/ip
## 127.0.0.1
```

```bash
$ httpbin :8080/github.png -c 'curl https://assets-cdn.github.com/images/modules/logos_page/GitHub-Mark.png | httpbin add body'

$ curl -O http://127.0.0.1:8080/github.png
## [github.png]
```

## Manual

```bash
httpbin get remote-addr
httpbin get ip
httpbin get content-length
httpbin get method
httpbin get url
httpbin get proto
httpbin get host
httpbin get header
httpbin get body
httpbin get form
httpbin get post-form

httpbin add header <k:v>
httpbin add body [body]

httpbin set code [code]
```

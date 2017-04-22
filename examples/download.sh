#!/bin/bash
httpbin add header "Content-Type:image/gif"
curl "https://www.baidu.com/images/logo.gif" | httpbin add body

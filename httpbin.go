package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"
	// "net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ChannelMessage struct {
	Id       string
	Action   string
	Item     string
	Addition string
}

type RequestData struct {
	request    *http.Request
	body       *bytes.Buffer
	headers    http.Header
	statusCode int
}

type RequestMap struct {
	data map[string]*RequestData
	lock *sync.RWMutex
}

func newRequestMap() *RequestMap {
	return &RequestMap{
		data: make(map[string]*RequestData),
		lock: &sync.RWMutex{},
	}
}

func (self *RequestMap) set(key string, value *RequestData) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.data[key] = value
}

func (self *RequestMap) get(key string) (*RequestData, bool) {
	self.lock.RLock()
	defer self.lock.RUnlock()
	v, ok := self.data[key]
	return v, ok
}

func (self *RequestMap) erase(key string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	delete(self.data, key)
}

var (
	serveMux   *http.ServeMux
	requestMap *RequestMap
	stdMutex   *sync.Mutex
)

var (
	listenPort    string
	listenPattern string
	scriptFile    string
	masterPattern string
)

const (
	letters                = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ENVNAME_REQUEST_ID     = "HTTPBIN_REQUEST_ID"
	ENVNAME_SERVER_ADDRESS = "HTTPBIN_SERVER_ADDRESS"
)

func randString() string {
	ts := []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	b := make([]byte, len(ts)*2)
	for i := range b {
		if i%2 == 0 {
			b[i] = letters[rand.Intn(len(letters))]
		} else {
			b[i] = ts[i/2]
		}
	}
	return string(b)
}

func Printf(format string, a ...interface{}) (n int, err error) {
	stdMutex.Lock()
	defer stdMutex.Unlock()
	return fmt.Printf(format+"\n", a...)
}

func Eprintf(format string, a ...interface{}) (n int, err error) {
	stdMutex.Lock()
	defer stdMutex.Unlock()
	return fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func PrintReader(src io.Reader) (written int64, err error) {
	stdMutex.Lock()
	defer stdMutex.Unlock()
	return io.Copy(os.Stdout, src)
}

func EprintReader(src io.Reader) (written int64, err error) {
	stdMutex.Lock()
	defer stdMutex.Unlock()
	return io.Copy(os.Stderr, src)
}

func Eprintln(a ...interface{}) (n int, err error) {
	stdMutex.Lock()
	defer stdMutex.Unlock()
	return fmt.Fprintln(os.Stderr, a...)
}

func callbackHandle(w http.ResponseWriter, r *http.Request) {
	requestId := randString()
	request := &RequestData{
		request:    r,
		body:       &bytes.Buffer{},
		headers:    w.Header(),
		statusCode: 200,
	}
	requestMap.set(requestId, request)
	execScript(scriptFile, requestId)

	w.WriteHeader(request.statusCode)
	io.Copy(w, request.body)

	requestMap.erase(requestId)
}

func masterHandle(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		Eprintf("fail to read body %v", err)
		return
	}

	msg := &ChannelMessage{}
	err = json.Unmarshal(body, msg)
	if err != nil {
		Eprintf("fail to parse body '%v', error %v", string(body), err)
		return
	}

	req, ok := requestMap.get(msg.Id)
	if !ok {
		Eprintf("not exist request '%v'", msg.Id)
		return
	}

	switch msg.Action {
	case "get":
		switch msg.Item {
		case "method":
			w.Write([]byte(req.request.Method))
			break
		case "url":
			w.Write([]byte(req.request.URL.String()))
			break
		case "proto":
			w.Write([]byte(req.request.Proto))
			break
		case "host":
			w.Write([]byte(req.request.Host))
			break
		case "header":
			if len(msg.Addition) == 0 {
				req.request.Header.Write(w)
			} else {
				w.Write([]byte(req.request.Header.Get(msg.Addition)))
			}
			break
		case "body":
			io.Copy(w, req.request.Body)
			break
		case "form":
			req.request.ParseForm()
			if len(msg.Addition) == 0 {
				w.Write([]byte(req.request.Form.Encode()))
			} else {
				w.Write([]byte(req.request.Form.Get(msg.Addition)))
			}
			break
		case "postform":
			req.request.ParseForm()
			if len(msg.Addition) == 0 {
				w.Write([]byte(req.request.PostForm.Encode()))
			} else {
				w.Write([]byte(req.request.PostForm.Get(msg.Addition)))
			}
			break
		}
		break
	case "add":
		switch msg.Item {
		case "header":
			head := strings.SplitN(msg.Addition, ":", 2)
			req.headers.Add(head[0], head[1])
			break
		case "body":
			req.body.WriteString(msg.Addition)
			break
		}
		break
	case "set":
		if msg.Item == "code" {
			code, ok := strconv.Atoi(msg.Addition)
			if ok != nil {
				req.statusCode = code
			}
		}
		break
	}
}

func ListenAndServe() error {
	serveMux.HandleFunc(listenPattern, callbackHandle)
	serveMux.HandleFunc(masterPattern, masterHandle)
	return http.ListenAndServe(listenPort, serveMux)
}

func setExecEnviron(cmd *exec.Cmd, key, value string) {
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
}

func execScript(scriptName string, requestId string) {
	cmd := exec.Command("bash", scriptName)
	cmd.Env = os.Environ()
	setExecEnviron(cmd, ENVNAME_REQUEST_ID, requestId)
	setExecEnviron(cmd, ENVNAME_SERVER_ADDRESS, fmt.Sprintf("%s:%s", listenPort, masterPattern))

	bar := "\033[0;30m====================\033[0m"

	buffer := &bytes.Buffer{}
	buffer.Write([]byte(fmt.Sprintf("%s %s %s\n", bar, requestId, bar)))

	cmd.Stdout = buffer
	cmd.Stderr = buffer

	err := cmd.Start()
	if err != nil {
		Eprintf("fail to execute command '%v'", err)
		return
	}
	cmd.Wait()

	buffer.Write([]byte(fmt.Sprintf("%s %s %s\n", bar, requestId, bar)))
	PrintReader(buffer)
}

func init() {
	rand.Seed(time.Now().UnixNano())
	serveMux = http.NewServeMux()
	requestMap = newRequestMap()
	stdMutex = &sync.Mutex{}
}

func printEmbedModeUsage() {
	Eprintf("Usage: %s <action> <item> [addition]", os.Args[0])
	Eprintf("Example:")
	Eprintf("\t%s get header Host", os.Args[0])
	Eprintf("\t%s add header Content-Type:application/json", os.Args[0])
	Eprintf("\t%s add body ok", os.Args[0])
}

func embedRun(requestId, serverAddress string) {
	if len(os.Args) < 3 {
		printEmbedModeUsage()
		os.Exit(1)
	}
	msg := &ChannelMessage{
		Id:     requestId,
		Action: os.Args[1],
		Item:   os.Args[2],
	}

	if len(os.Args) > 3 {
		msg.Addition = os.Args[3]
	}

	body, err := json.Marshal(msg)
	if err != nil {
		Eprintf("fail to encode msg '%v' error '%v'", msg, err)
		os.Exit(1)
	}

	if serverAddress[0] == ':' {
		serverAddress = "http://127.0.0.1" + serverAddress
	} else {
		serverAddress = "http://" + serverAddress
	}

	resp, err := http.Post(serverAddress, "application/json", strings.NewReader(string(body)))
	if err != nil {
		Eprintf("fail to post request '%v'", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		_, err = PrintReader(resp.Body)
		if err != nil {
			Eprintf("fail to read body '%v'", err)
			os.Exit(1)
		}
		os.Exit(0)
	} else {
		_, err = EprintReader(resp.Body)
		if err != nil {
			Eprintf("fail to read body '%v'", err)
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func printUsage() {
	Eprintf("Usage: %s <adress> <script-file>", os.Args[0])
	Eprintf("Example:")
	Eprintf("\t%s :8080/callback cb.sh", os.Args[0])
}

func main() {

	requestId := os.Getenv(ENVNAME_REQUEST_ID)
	serverAddress := os.Getenv(ENVNAME_SERVER_ADDRESS)
	if len(requestId) != 0 && len(serverAddress) != 0 {
		embedRun(requestId, serverAddress)
		return
	}

	if len(os.Args) != 3 {
		printUsage()
		os.Exit(1)
	}

	address := strings.SplitN(os.Args[1], "/", 2)
	if len(address) != 2 {
		Eprintf("unexpected adderss format '%v'", os.Args[1])
		os.Exit(1)
	}

	listenPort = address[0]
	listenPattern = "/" + address[1]
	scriptFile = os.Args[2]
	masterPattern = "/" + randString()

	if fileInfo, err := os.Stat(scriptFile); os.IsNotExist(err) || fileInfo.IsDir() {
		Eprintf("script '%v' does not exist", scriptFile)
		return
	}
	Printf("serve '%v' pattern '%v' scriptName '%v'", listenPort, listenPattern, scriptFile)

	err := ListenAndServe()
	if err != nil {
		Eprintf("fail to listen and serve: %v", err)
		os.Exit(1)
	}
}

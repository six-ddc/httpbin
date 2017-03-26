package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
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

var (
	msgChan    chan *ChannelMessage
	serveMux   *http.ServeMux
	requestMap map[string]*http.Request
)

var (
	listenPort    string
	listenPattern string
	scriptFile    string
	masterPattern string
)

const (
	letters                = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ENVNAME_REQUEST_ID     = "HTTPCB_REQUEST_ID"
	ENVNAME_SERVER_ADDRESS = "HTTPCB_SERVER_ADDRESS"
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

func callbackHandle(w http.ResponseWriter, r *http.Request) {
	requestId := randString()
	requestMap[requestId] = r
	go execScript(scriptFile, requestId)
	body := &bytes.Buffer{}
	for {
		msg := <-msgChan
		if msg.Id != requestId {
			msgChan <- msg
			continue
		}
		if msg.Action == "end" {
			delete(requestMap, requestId)
			break
		}
		switch msg.Action {
		case "add":
			switch msg.Item {
			case "header":
				head := strings.SplitN(msg.Addition, ":", 2)
				w.Header().Add(head[0], head[1])
				break
			case "body":
				body.WriteString(msg.Addition)
				break
			}
			break
		case "set":
			if msg.Item == "code" {
				code, _ := strconv.Atoi(msg.Addition)
				w.WriteHeader(code)
			}
		}
	}
	io.Copy(w, body)
}

func masterHandle(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to read body %v\n", err)
		return
	}
	msg := &ChannelMessage{}
	err = json.Unmarshal(body, msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to parse body '%v', error %v\n", string(body), err)
		return
	}
	// fmt.Printf("recv msg %+v\n", msg)
	switch msg.Action {
	case "get":
		if request, ok := requestMap[msg.Id]; ok {
			switch msg.Item {
			case "header":
				if len(msg.Addition) == 0 {
					request.Header.Write(w)
				} else {
					w.Write([]byte(request.Header.Get(msg.Addition)))
				}
				break
			case "body":
				io.Copy(w, request.Body)
				break
			case "url":
				w.Write([]byte(request.URL.String()))
				break
			case "host":
				w.Write([]byte(request.Host))
				break
			case "method":
				w.Write([]byte(request.Method))
				break
			case "form":
				request.ParseForm()
				if len(msg.Addition) == 0 {
					w.Write([]byte(request.Form.Encode()))
				} else {
					w.Write([]byte(request.Form.Get(msg.Addition)))
				}
				break
			case "postform":
				request.ParseForm()
				if len(msg.Addition) == 0 {
					w.Write([]byte(request.PostForm.Encode()))
				} else {
					w.Write([]byte(request.PostForm.Get(msg.Addition)))
				}
				break
			}
		}
		break
	case "add":
		msgChan <- msg
		break
	case "set":
		msgChan <- msg
		break
	}
}

func newServe() error {
	serveMux.HandleFunc(listenPattern, callbackHandle)
	serveMux.HandleFunc(masterPattern, masterHandle)
	err := http.ListenAndServe(listenPort, serveMux)
	if err != nil {
		return fmt.Errorf("fail to serve: %v", err)
	}
	return nil
}

func setExecEnviron(cmd *exec.Cmd, key, value string) {
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
}

func execScript(scriptName string, requestId string) {
	cmd := exec.Command("bash", scriptName)
	cmd.Env = os.Environ()
	setExecEnviron(cmd, ENVNAME_REQUEST_ID, requestId)
	setExecEnviron(cmd, ENVNAME_SERVER_ADDRESS, fmt.Sprintf("%s:%s", listenPort, masterPattern))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer stdout.Close()
	stdoutBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Println(string(stdoutBytes))
	msg := &ChannelMessage{
		Id:     requestId,
		Action: "end",
	}
	msgChan <- msg
}

func init() {
	rand.Seed(time.Now().UnixNano())
	msgChan = make(chan *ChannelMessage)
	serveMux = http.NewServeMux()
	requestMap = make(map[string]*http.Request)
}

func printEmbedModeUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <action> <item> [addition]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Example:\n")
	fmt.Fprintf(os.Stderr, "\t%s get header Host\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\t%s add header Content-Type:application/json\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\t%s add body ok\n", os.Args[0])
}

func embedMode(requestId, serverAddress string) {
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
		fmt.Fprintf(os.Stderr, "fail to encode msg '%v' error '%v'\n", msg, err)
		os.Exit(1)
	}

	if serverAddress[0] == ':' {
		serverAddress = "http://127.0.0.1" + serverAddress
	} else {
		serverAddress = "http://" + serverAddress
	}

	resp, err := http.Post(serverAddress, "application/json", strings.NewReader(string(body)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to post request '%v'\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to read body '%v'\n", err)
		os.Exit(1)
	}
	fmt.Printf(string(body))
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <adress> <script-file>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Example:\n")
	fmt.Fprintf(os.Stderr, "\t%s :8080/callback cb.sh\n", os.Args[0])
}

func main() {

	requestId := os.Getenv(ENVNAME_REQUEST_ID)
	serverAddress := os.Getenv(ENVNAME_SERVER_ADDRESS)
	if len(requestId) != 0 && len(serverAddress) != 0 {
		embedMode(requestId, serverAddress)
		return
	}

	if len(os.Args) != 3 {
		printUsage()
		os.Exit(1)
	}

	address := strings.SplitN(os.Args[1], "/", 2)
	if len(address) != 2 {
		fmt.Fprintf(os.Stderr, "unexpected adderss format '%v'\n", os.Args[1])
		os.Exit(1)
	}

	listenPort = address[0]
	listenPattern = "/" + address[1]
	scriptFile = os.Args[2]
	masterPattern = "/" + randString()

	if fileInfo, err := os.Stat(scriptFile); os.IsNotExist(err) || fileInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "script '%v' does not exist\n", scriptFile)
		return
	}
	fmt.Printf("serve '%v' pattern '%v' scriptName '%v'\n", listenPort, listenPattern, scriptFile)

	err := newServe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

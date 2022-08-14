package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

func must(msg string, err error) {
	if err != nil {
		panic("failed to " + msg + ": " + err.Error())
	}
}

func errorLog(msg string, err error) {
	log.Printf("[ERROR] failed to %s: %v", msg, err)
}

func infoLog(msg string) {
	log.Printf("[INFO] %s", msg)
}

func main() {
	l, err := net.Listen("tcp", ":3000")
	must("listen on :3000", err)

	infoLog("starting server, listen on :3000")
	for {
		infoLog("start listening...")
		conn, err := l.Accept()
		if err != nil {
			errorLog("accept connection", err)
			continue
		}

		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	infoLog("start processing connection")

	r := bufio.NewReader(conn)
	method, requestURI, proto, err := parseRequestLine(r)
	if err != nil {
		errorLog("parse request line", err)
		return
	}

	header, err := parseMIMEHeader(r)
	if err != nil {
		errorLog("parse MIME header", err)
		return
	}
	contentLength, err := parseContentLength(header)
	if err != nil {
		errorLog("parse Content-Length", err)
		return
	}

	// construct Request object
	req := Request{
		RemoteAddr: conn.RemoteAddr().String(),
		Method:     method,
		RequestURI: requestURI,
		Proto:      proto,
		Header:     header,
		Body: struct {
			io.Reader
			io.Closer
		}{io.LimitReader(r, contentLength), ioutil.NopCloser(r)},
	}

	resp := Response{}
	handlerFn(&req, &resp)

	if _, err := conn.Write(resp.respond()); err != nil {
		errorLog("write response", err)
	}
	infoLog("end of connection")
}

func parseRequestLine(r *bufio.Reader) (method, requestURI, proto string, err error) {
	// First line: GET /index.html HTTP/1.0
	line, err := r.ReadString('\n')
	if err != nil {
		return "", "", "", err
	}

	method, rest, ok1 := strings.Cut(line, " ")
	requestURI, proto, ok2 := strings.Cut(rest, " ")
	if !ok1 || !ok2 {
		return "", "", "", errors.New("invalid request line")
	}
	return method, requestURI, proto, nil
}

func parseMIMEHeader(r *bufio.Reader) (header http.Header, err error) {
	header = make(http.Header)

	for {
		kv, err := r.ReadString('\n')
		if err != nil {
			return header, err
		}

		kv = strings.TrimSpace(kv)
		if len(kv) == 0 {
			return header, err
		}

		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			return header, fmt.Errorf("invalid header: %s", kv)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		header[k] = append(header[k], v)
	}
}

func parseContentLength(h http.Header) (int64, error) {
	cl := h.Get("Content-Length")
	if len(cl) == 0 {
		if len(h["content-length"]) > 0 {
			cl = h["content-length"][0]
		}
	}

	cl = textproto.TrimString(cl)
	if cl == "" {
		return -1, nil
	}
	n, err := strconv.ParseUint(cl, 10, 63)
	if err != nil {
		return 0, fmt.Errorf("bad Content-Length: %s", cl)
	}
	return int64(n), nil
}

func handlerFn(req *Request, resp *Response) {
	fmt.Println(req.RemoteAddr)
	fmt.Println(req.Method)
	fmt.Println(req.RequestURI)
	fmt.Println(req.Header)

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(body))
	}

	resp.WriteStatus(http.StatusOK)
	resp.WriteHeader("Content-Type", "text/plain")
	resp.WriteData([]byte("hello world"))
}

type Request struct {
	RemoteAddr string
	Method     string
	RequestURI string
	Proto      string
	Header     http.Header
	Body       io.ReadCloser
}

type Response struct {
	status int
	header http.Header
	data   []byte
}

func (r *Response) WriteStatus(code int) {
	if code < 100 || code > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %v", code))
	}
	r.status = code
}

func (r *Response) WriteData(data []byte) { r.data = append(r.data, data...) }

func (r *Response) WriteHeader(field, value string) {
	if r.header == nil {
		r.header = make(http.Header)
	}
	r.header[field] = append(r.header[field], value)
}

func (r *Response) respond() []byte {
	statusLine := fmt.Sprintf(`HTTP/1.1 %v OK`, r.status)
	headers := make([]string, 0, len(r.header))
	for k, v := range r.header {
		for _, vv := range v {
			headers = append(headers, fmt.Sprintf("%s: %s", k, vv))
		}
	}
	headers = append(headers, fmt.Sprintf("Content-Length: %v", len(r.data)))

	return append([]byte(
		statusLine+"\n"+
			strings.Join(headers, "\n")+"\n"+
			"\n", // new line between header and body
	), r.data...)
}

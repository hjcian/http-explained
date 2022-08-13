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

func main() {
	l, err := net.Listen("tcp", ":3000")
	must("listen on :3000", err)

	log.Println("starting server, listen on :3000")
	for {
		log.Println("start listening...")
		conn, err := l.Accept()
		must("accept connection", err)
		go handleConn(conn)
	}
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

func handleConn(conn net.Conn) {
	log.Println("---- get connection from: ", conn.RemoteAddr(), " ----")

	r := bufio.NewReader(conn)

	method, requestURI, proto, err := parseRequestLine(r)
	if err != nil {
		errorLog("parse request line", err)
		return
	}
	headers, err := parseMIMEHeader(r)
	if err != nil {
		errorLog("parse MIME header", err)
		return
	}

	req := Request{
		RemoteAddr: conn.RemoteAddr().String(),
		Method:     method,
		RequestURI: requestURI,
		Proto:      proto,
		Header:     headers,
	}

	length, err := parseContentLength(req.Header)
	if err != nil {
		errorLog("parse Content-Length", err)
		return
	}
	req.Body = makeBodyReadCloser(r, length)

	resp := Response{}
	handlerFn(&req, &resp)

	n, err := conn.Write(resp.respond())
	log.Println("---- end of connection ---", n, err)
	if err := conn.Close(); err != nil {
		errorLog("close connection", err)
	}
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

func parseMIMEHeader(r *bufio.Reader) (headers map[string][]string, err error) {
	headers = make(map[string][]string)

	for {
		kv, err := r.ReadString('\n')
		if err != nil {
			return headers, err
		}

		kv = strings.TrimSpace(kv)
		if len(kv) == 0 {
			return headers, err
		}

		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			return headers, fmt.Errorf("invalid header: %s", kv)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		headers[k] = append(headers[k], v)
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

type naiveCloser struct {
	r *bufio.Reader
}

func (c *naiveCloser) Close() error {
	if c.r.Buffered() > 0 {
		_, err := io.Copy(io.Discard, c.r)
		return err
	}
	return nil
}

func makeBodyReadCloser(r *bufio.Reader, contentLength int64) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{
		io.LimitReader(r, contentLength),
		&naiveCloser{r},
	}
}

func handlerFn(req *Request, resp *Response) {
	fmt.Println(req.RemoteAddr)
	fmt.Println(req.Method)
	fmt.Println(req.RequestURI)
	fmt.Println(req.Header)

	body, err := ioutil.ReadAll(req.Body)

	fmt.Println(err)
	fmt.Println(string(body))

	resp.WriteStatus(http.StatusOK)
	resp.WriteHeader("Content-Type", "text/plain")
	resp.WriteData([]byte("hello world"))
}

package fzf

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	crlf           = "\r\n"
	httpOk         = "HTTP/1.1 200 OK" + crlf
	httpBadRequest = "HTTP/1.1 400 Bad Request" + crlf
)

func startHttpServer(port int, channel chan []*action) error {
	if port == 0 {
		return nil
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("port not available: %d", port)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break
				} else {
					continue
				}
			}
			conn.Write([]byte(handleHttpRequest(conn, channel)))
			conn.Close()
		}
		listener.Close()
	}()

	return nil
}

// Here we are writing a simplistic HTTP server without using net/http
// package to reduce the size of the binary.
//
// * No --listen:            2.8MB
// * --listen with net/http: 5.7MB
// * --listen w/o net/http:  3.3MB
func handleHttpRequest(conn net.Conn, channel chan []*action) string {
	line := 0
	headerRead := false
	contentLength := 0
	body := ""
	bad := func(message string) string {
		message += "\n"
		return httpBadRequest + fmt.Sprintf("Content-Length: %d%s", len(message), crlf+crlf+message)
	}
	scanner := bufio.NewScanner(conn)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		found := bytes.Index(data, []byte(crlf))
		if found >= 0 {
			token := data[:found+len(crlf)]
			return len(token), token, nil
		}
		if atEOF || len(body)+len(data) >= contentLength {
			return 0, data, bufio.ErrFinalToken
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		text := scanner.Text()
		if line == 0 && !strings.HasPrefix(text, "POST / HTTP") {
			return bad("invalid request method")
		}
		if text == crlf {
			headerRead = true
		}
		if !headerRead {
			pair := strings.SplitN(text, ":", 2)
			if len(pair) == 2 && strings.ToLower(pair[0]) == "content-length" {
				length, err := strconv.Atoi(strings.TrimSpace(pair[1]))
				if err != nil {
					return bad("invalid content length")
				}
				contentLength = length
			}
		} else if contentLength <= 0 {
			break
		} else {
			body += text
		}
		line++
	}

	errorMessage := ""
	actions := parseSingleActionList(strings.Trim(string(body), "\r\n"), func(message string) {
		errorMessage = message
	})
	if len(errorMessage) > 0 {
		return bad(errorMessage)
	}
	if len(actions) == 0 {
		return bad("no action specified")
	}

	channel <- actions
	return httpOk
}

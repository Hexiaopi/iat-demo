package iat

import (
	"flag"
	"io"
	"os"
	"testing"
)

var key = flag.String("key", "", "iflytek iat key")
var secret = flag.String("secret", "", "iflytek iat secret")
var appid = flag.String("appid", "", "iflytek iat appid")

func TestIflytek(t *testing.T) {
	flag.Parse()
	iat := NewIflytek(*key, *secret, *appid)
	conn, err := iat.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	result := make(chan string)
	go iat.Receive(conn, result)

	audio := make(chan []byte)
	go iat.Send(conn, audio)

	file, err := os.Open("./testdata/test.pcm")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for {
		buffer := make([]byte, 1024)
		len, err := file.Read(buffer)
		t.Log("read", len)
		if err != nil {
			if err == io.EOF {
				iat.Close()
			}
			break
		}
		audio <- buffer[:len]
	}
	for text := range result {
		t.Log(text)
	}
}

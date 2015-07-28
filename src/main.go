package main

import (
	"bufio"
	"log"
	"net/http"
	//"fmt"  // DEBUG
	"EvLJson"
)

/*

Lets assume that there is copper somewhere between the target and us:

-
http://stackoverflow.com/questions/2613734/maximum-packet-size-for-a-tcp-connection
-

The absolute limitation on TCP packet size is 64K (65535 bytes), but in practicality this is far larger than the size of any packet you will see, because the lower layers (e.g. ethernet) have lower packet sizes. The MTU (Maximum Transmission Unit) for Ethernet, for instance, is 1500 bytes.


default buffer size as of 7/24/2015:

-
http://golang.org/src/bufio/bufio.go?s=1648:1684#L51
-

const (
    defaultBufSize = 4096
)


instagram post size limits:
-
http://www.jennstrends.com/limits-on-instagram/
-
2200 characters, possibly 6 byte utf-8 encoding

*/

const TCP_HEADER_SIZE = 40
const BUFIO_READER_SIZE = 1500 - TCP_HEADER_SIZE // between ( 1024 = 2 ^ 10 ) and ( 2048 = 2 ^ 11 )
const LITERAL_BUFF_SIZE = 13200                  // 13200 ( 2200 * 6 )

func main() {

	httpResponse, err := http.Get("http://127.0.0.1:8080")

	if err != nil {
		log.Fatal(err)
	}

	bufferedReader := bufio.NewReaderSize(httpResponse.Body, BUFIO_READER_SIZE)
	evLJsonParser := EvLJson.NewParser()

	err = evLJsonParser.Parse(bufferedReader)

	if err != nil {
		log.Fatal(err)
	}

	defer httpResponse.Body.Close()
}

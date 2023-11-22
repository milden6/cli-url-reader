package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"
)

const (
	queueSize      = 100
	requestTimeout = time.Second * 5

	retryDelay = time.Millisecond * 200
)

var (
	httpClient  *http.Client
	validURLReg *regexp.Regexp
)

// args
var (
	maxRetries int
	maxWorkers int
	inputFile  string
)

func main() {
	flag.IntVar(&maxRetries, "maxretries", 3, "Max number of request retries")
	flag.IntVar(&maxWorkers, "maxworkers", 10, "Workers count")
	flag.StringVar(&inputFile, "input", "input.txt", "Input file path")

	flag.Parse()

	validURLReg = regexp.MustCompile(`^((http[s]?):\/\/)?(www.)?[a-z0-9]+\.[a-z]+(\/[a-zA-Z0-9#]+\/?)*$`)

	httpClient = &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Second * 1,
			}).DialContext,
			TLSHandshakeTimeout:   time.Second * 1,
			ResponseHeaderTimeout: time.Second * 1,
		},
	}

	urlQueue := make(chan string, queueSize)

	wg := sync.WaitGroup{}

	// run workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for url := range urlQueue {
				doRequest(url)
			}
		}()
	}

	// read file
	f, err := os.Open(inputFile)
	if err != nil {
		log.Fatalf("failed to open file %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Println("failed to close file")
		}
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		urlQueue <- scanner.Text()
	}
	close(urlQueue)

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	wg.Wait()
}

func doRequest(url string) {
	if !validURLReg.MatchString(url) {
		log.Printf("invalid url %v: \n", url)
		return
	}

	startProcessing := time.Now()

	// do retries
	attempt := 0
	for {
		attempt++

		if attempt > maxRetries {
			log.Printf("failed all retries for url: %v\n", url)
			return
		}

		res, err := httpClient.Get(url)
		if err != nil {
			log.Printf("failed to send request with url %v: err %v \n", url, err)

			time.Sleep(retryDelay)
			continue
		}

		if res.StatusCode != http.StatusOK {
			log.Printf("response failed with status code: %d and url: %v\n", res.StatusCode, url)

			time.Sleep(retryDelay)
			continue
		}

		body, err := io.ReadAll(res.Body)
		if err := res.Body.Close(); err != nil {
			log.Printf("failed to close body for url: %v\n", url)
			return
		}

		if err != nil {
			log.Printf("failed to read response for url: %v\n", url)
			return
		}

		fmt.Printf("Size of {%v}: %d bytes\n", url, len(body))
		fmt.Printf("Processing time for {%v}: %s\n", url, time.Since(startProcessing))
		fmt.Println()

		break
	}
}

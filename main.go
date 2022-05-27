package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/michael1026/sessionManager"
)

var (
	resources   SafeResources
	jsonResults map[string]string
	client      *http.Client
	threads     int
)

type CookieInfo map[string]string

type SafeResources struct {
	mu        sync.Mutex
	resources map[string]bool
}

type Response struct {
	*http.Response
	url string
	err error
}

type Request struct {
	*http.Request
	url string
}

func (c *SafeResources) AddResource(key string) {
	c.mu.Lock()
	c.resources[key] = true
	c.mu.Unlock()
}

func (c *SafeResources) AddAndPrintIfUnique(key string, url string, contentType string) {
	c.mu.Lock()
	if !c.resources[key] {
		fmt.Println(url)
		c.resources[key] = true
		jsonResults[url] = contentType
	}
	c.mu.Unlock()
}

func (c *SafeResources) Value(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resources[key]
}

func buildHttpClient(jar *cookiejar.Jar) (c *http.Client) {
	transport := &http.Transport{
		MaxIdleConns:      30,
		IdleConnTimeout:   3 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: time.Second,
		}).DialContext,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 8,
		Jar:           jar,
	}

	return client
}

func main() {
	resources = SafeResources{resources: make(map[string]bool)}
	cookieFile := flag.String("C", "", "File containing cookie")
	flag.IntVar(&threads, "t", 5, "Number of concurrent threads")
	outputJson := flag.String("json", "", "Output as json")
	jsonResults = make(map[string]string)

	flag.Parse()

	jar := sessionManager.ReadCookieJson(*cookieFile)
	urls := []string{}

	client = buildHttpClient(jar)

	wg := sync.WaitGroup{}
	s := bufio.NewScanner(os.Stdin)

	for s.Scan() {
		urls = append(urls, s.Text())
	}

	reqChan := make(chan Request)
	respChan := make(chan Response)

	go dispatcher(urls, reqChan)
	go workerPool(reqChan, respChan)
	consumer(urls, respChan)

	wg.Wait()

	if *outputJson != "" {
		jsonFile, err := json.Marshal(jsonResults)

		if err != nil {
			fmt.Printf("Error marshalling JSON: %s\n", err)
			return
		}

		err = ioutil.WriteFile(*outputJson, jsonFile, 0644)

		if err != nil {
			fmt.Printf("Error writing JSON to file: %s\n", err)
		}
	}
}

func printUniqueContentURLs(resp http.Response, rawUrl string) {
	if resp.StatusCode == http.StatusOK {
		resource := ""
		if len(resp.Header.Get("content-type")) >= 9 && resp.Header.Get("content-type")[:9] == "text/html" {
			doc, err := goquery.NewDocumentFromReader(resp.Body)

			if err != nil {
				return
			}

			doc.Find("script[src]").Each(func(index int, item *goquery.Selection) {
				src, _ := item.Attr("src")
				srcurl, err := url.Parse(src)

				if err != nil {
					return
				}

				srcurl.RawQuery = ""

				resource += srcurl.String()
			})

			resources.AddAndPrintIfUnique(resource, rawUrl, "text/html")
		} else if len(resp.Header.Get("content-type")) >= 16 && resp.Header.Get("content-type")[:16] == "application/json" {
			var resultMap map[string]interface{}
			body, err := ioutil.ReadAll(resp.Body)

			if err != nil {
				return
			}

			err = json.Unmarshal([]byte(body), &resultMap)

			if err != nil {
				return
			}

			resource = mapKeysToString(resultMap)

			resources.AddAndPrintIfUnique(resource, rawUrl, "application/json")
		}
	}
}

func mapKeysToString(jsonMap map[string]interface{}) string {
	finalString := ""
	for k := range jsonMap {
		finalString += k
	}
	return finalString
}

func dispatcher(urls []string, reqChan chan Request) {
	defer close(reqChan)
	for _, url := range urls {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println(err)
			continue
		}
		req.Close = true
		req.Header.Add("Connection", "close")
		req.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0")
		req.Header.Add("Accept-Language", "en-US,en;q=0.9")
		req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")

		reqChan <- Request{req, url}
	}
}

func workerPool(reqChan chan Request, respChan chan Response) {
	for i := 0; i < threads; i++ {
		go worker(reqChan, respChan)
	}
}

func worker(reqChan chan Request, respChan chan Response) {
	for req := range reqChan {
		resp, err := client.Do(req.Request)
		r := Response{resp, req.url, err}
		respChan <- r
	}
}

func consumer(urls []string, respChan chan Response) {
	var (
		conns int64
	)

	for conns < int64(len(urls)) {
		select {
		case r, ok := <-respChan:
			if ok {
				if r.err == nil {
					printUniqueContentURLs(*r.Response, r.url)
					r.Body.Close()
				}
				conns++
			}
		}
	}
}

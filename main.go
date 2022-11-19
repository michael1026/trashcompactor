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
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/michael1026/sessionManager"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	urlMap      cmap.ConcurrentMap[string, bool]
	jsonResults cmap.ConcurrentMap[string, string]
	client      *http.Client
	threads     int
)

type CookieInfo map[string]string

type Response struct {
	*http.Response
	url string
	err error
}

type Request struct {
	*http.Request
	url string
}

func AddAndPrintIfUnique(urlMap cmap.ConcurrentMap[string, bool], key string, url string, contentType string) {
	if _, ok := urlMap.Get(key); !ok {
		fmt.Println(url)
		urlMap.Set(key, true)
		jsonResults.Set(url, contentType)
	}
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
	urlMap = cmap.New[bool]()
	cookieFile := flag.String("C", "", "File containing cookie")
	flag.IntVar(&threads, "t", 5, "Number of concurrent threads")
	outputJson := flag.String("json", "", "Output as json")
	jsonResults = cmap.New[string]()

	flag.Parse()

	jar := sessionManager.ReadCookieJson(*cookieFile)
	urls := []string{}

	client = buildHttpClient(jar)

	s := bufio.NewScanner(os.Stdin)

	for s.Scan() {
		urls = append(urls, s.Text())
	}

	reqChan := make(chan Request)
	respChan := make(chan Response)
	done := make(chan bool)

	go dispatcher(urls, reqChan)
	go workerPool(reqChan, respChan)
	go consumer(urls, respChan, done)
	<-done

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

			AddAndPrintIfUnique(urlMap, resource, rawUrl, "text/html")
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

			AddAndPrintIfUnique(urlMap, resource, rawUrl, "application/json")
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
	close(respChan)
}

func consumer(urls []string, respChan chan Response, done chan bool) {
	for resp := range respChan {
		if resp.Response != nil {
			printUniqueContentURLs(*resp.Response, resp.url)
		}
	}
	done <- true
}

package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/michael1026/sessionManager"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/projectdiscovery/fastdialer/fastdialer"
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
	fastdialerOpts := fastdialer.DefaultOptions
	fastdialerOpts.EnableFallback = true
	dialer, err := fastdialer.NewDialer(fastdialerOpts)
	if err != nil {
		log.Fatal("Error building HTTP client")
		return nil
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     time.Second * 10,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			Renegotiation:      tls.RenegotiateOnceAsClient,
		},
		DisableKeepAlives: false,
		DialContext:       dialer.Dial,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 5,
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
	done := make(chan bool)

	go producer(urls, reqChan)
	for i := 0; i < threads; i++ {
		go consumer(reqChan, done)
	}
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

func producer(urls []string, reqChan chan Request) {
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

func consumer(reqChan chan Request, done chan bool) {
	for req := range reqChan {
		resp, err := client.Do(req.Request)
		r := Response{resp, req.url, err}
		if r.Response != nil {
			printUniqueContentURLs(*r.Response, r.url)
		}
	}
	done <- true
}

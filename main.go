package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
)

var client *alidns.Client

func main() {
	logPath := "./logs"
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		err = os.Mkdir(logPath, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
	file, err := os.OpenFile("logs/ddns.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	log.SetOutput(file)

	log.Printf("INFO|Starting Aliyun ddns...")
	ak, err := loadAccessKey()
	if err != nil {
		log.Fatalf("ERROR|LoadAccessKey failed: %s", err)
	}
	client, err = alidns.NewClientWithAccessKey("cn-hangzhou", ak.AccessKey, ak.AccessKeySecret)
	if err != nil {
		log.Fatalf("ERROR|Open alidns client failed: %s", err)
	}
	defer client.Shutdown()
	log.Printf("INFO|Aliyun ddns is running")

	failedCnt := 0
	for {
		nextTimeout := time.Duration(ak.IntervalMinutes) * time.Minute
		if startDDNS() {
			failedCnt = 0
		} else {
			failedCnt++
			if failedCnt >= 3 {
				log.Printf("ERROR|Failed %d times, do not retry anymore", failedCnt)
				failedCnt = 0
			} else {
				log.Printf("ERROR|Failed %d times, retry after 10 seconds", failedCnt)
				nextTimeout = time.Duration(10) * time.Second
			}
		}
		time.Sleep(nextTimeout)
	}
}

func loadRecords() (*dnsRecordPack, error) {
	file, err := os.Open("config/records.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var records dnsRecordPack
	err = dec.Decode(&records)
	if err != nil {
		return nil, err
	}
	return &records, nil
}

func loadAccessKey() (*accessKey, error) {
	file, err := os.Open("config/accessKey.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var ak accessKey
	err = dec.Decode(&ak)
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

func startDDNS() bool {
	pack, err := loadRecords()
	if err != nil {
		log.Printf("ERROR|Load records failed: %s", err)
		return false
	}
	success := true

	ipv4, ipv6 := fetchIp()
	if ipv4 == "" && ipv6 == "" {
		log.Println("ERROR|Could not get ip!")
		return false
	}

	for _, r := range pack.Locals {
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|GetAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			success = false
			continue
		}
		ip := ipv4
		if r.Type == "AAAA" {
			ip = ipv6
		}
		if ip != val {
			r.Value = ip
			success = setRecord(r) && success
		}
	}

	if ipv6 == "" {
		if len(pack.Prefixes) > 0 {
			log.Println("ERROR|Could not find ipv6, ignore prefixes.")
		}
		return success
	}

	for _, r := range pack.Prefixes {
		if r.Type != "AAAA" {
			continue
		}
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|GetAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			success = false
			continue
		}
		prefix := getPrefix(r.Prefix, ipv6)
		if prefix == "" {
			log.Printf("ERROR|Could not get new prefix, %d, %s", r.Prefix, ipv6)
			success = false
			continue
		}
		ip := prefix + r.Suffix
		if ip != val {
			r.Value = ip
			success = setRecord(r) && success
		}
	}

	return success
}

func getPrefix(prefix int32, ipv6 string) string {
	if ipv6 == "" {
		return ""
	}
	arr := strings.Split(ipv6, ":")
	var p string
	for i := 0; i < int(prefix); i++ {
		p = p + arr[i] + ":"
	}
	return p
}

func setRecord(record *dnsRecord) bool {
	if record.Value == "" {
		log.Printf("WARN|Update alidns no val, ignore: [%s|%s|%s]", record.Type, record.Host, record.Value)
		return true
	}

	request := alidns.CreateUpdateDomainRecordRequest()
	request.RecordId = record.RecordId
	request.RR = record.RR
	request.Type = record.Type
	request.Value = record.Value
	request.Lang = "en"
	request.Priority = "1"
	request.TTL = "600"
	request.Line = "default"

	resp, err := client.UpdateDomainRecord(request)
	if err != nil {
		log.Printf("ERROR|Update alidns failed: %s, %v", err.Error(), resp)
		return false
	} else {
		log.Printf("INFO|Update alidns:%v, [%s|%s|%s]", resp.IsSuccess(), record.Type, record.Host, record.Value)
		return true
	}
}

func getAliRecordValue(recordId string) (string, error) {
	request := alidns.CreateDescribeDomainRecordInfoRequest()
	request.Lang = "en"
	request.RecordId = recordId
	resp, err := client.DescribeDomainRecordInfo(request)
	if err != nil {
		return "", err
	}
	return resp.Value, nil
}

type dnsRecordPack struct {
	Locals   []*dnsRecord `json:"locals"`
	Prefixes []*dnsRecord `json:"prefixes"`
}
type dnsRecord struct {
	RecordId string `json:"recordId"`
	Type     string `json:"type"`
	RR       string `json:"rr"`
	Value    string `json:"value"`
	Host     string `json:"host"`
	Prefix   int32  `json:"prefix"`
	Suffix   string `json:"suffix"`
}
type accessKey struct {
	AccessKey       string `json:"accessKey"`
	AccessKeySecret string `json:"accessKeySecret"`
	IntervalMinutes int32  `json:"intervalMinutes"`
}

type ipQueryResult struct {
	IP        string `json:"IP"`
	IPVersion string `json:"IPVersion"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Result    bool   `json:"result"`
}

func fetchIp() (string, string) {
	ipv4 := findPublicIp("https://4.ipw.cn/api/ip/myip?json")
	ipv6 := findPublicIp("https://6.ipw.cn/api/ip/myip?json")
	return ipv4, ipv6
}

func findPublicIp(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("ERROR|Fetch ip http failed, url: %s", url)
		return ""
	}

	jd := json.NewDecoder(resp.Body)
	rlt := ipQueryResult{}
	err = jd.Decode(&rlt)
	if err != nil {
		log.Printf("ERROR|Fetch ip decode failed: %s", err)
		return ""
	}
	if rlt.Code == "querySuccess" {
		return rlt.IP
	} else {
		log.Printf("ERROR|Fetch ip failed: %s", rlt.Message)
		return ""
	}
}

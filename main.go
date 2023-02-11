package main

import (
	"encoding/json"
	"log"
	"net"
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
		log.Fatalf("ERROR|loadAccessKey failed: %s", err)
	}
	client, err = alidns.NewClientWithAccessKey("cn-hangzhou", ak.AccessKey, ak.AccessKeySecret)
	if err != nil {
		log.Fatalf("ERROR|open alidns client failed: %s", err)
	}
	defer client.Shutdown()
	log.Printf("INFO|Aliyun ddns is running")

	failedCnt := 0
	for {
		if startDDNS() {
			failedCnt = 0
			time.Sleep(time.Duration(ak.IntervalMinutes) * time.Minute)
		} else {
			failedCnt++
			log.Printf("INFO|retry for failed: %d", failedCnt)
			if failedCnt >= 3 {
				// if failed 3 times, do not retry anymore
				failedCnt = 0
				time.Sleep(time.Duration(ak.IntervalMinutes) * time.Minute)
			} else {
				// if failed, retry after 10 seconds
				time.Sleep(time.Duration(10) * time.Second)
			}
		}
	}
}

func getIp(rType string) string {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		log.Printf("ERROR|get ip failed: %v", err)
	}

	for _, address := range addrs {
		// 检查ip地址判断是否回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if rType == "AAAA" {
				if ipnet.IP.To16() != nil {
					return ipnet.IP.String()
				}
			} else {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}
	return ""
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
		log.Printf("ERROR|load records failed: %s", err)
		return false
	}
	success := true
	for _, r := range pack.Locals {
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|getAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			success = false
			continue
		}
		ip := getIp(r.Type)
		if ip != val {
			r.Value = ip
			success = setRecord(r) && success
		}
	}

	for _, r := range pack.Prfixes {
		if r.Type != "AAAA" {
			continue
		}
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|getAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			success = false
			continue
		}
		prefix := getPrefix(r.Prefix)
		ip := prefix + r.Suffix
		if ip != val {
			r.Value = ip
			success = setRecord(r) && success
		}
	}

	return success
}

func getPrefix(prefix int32) string {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		log.Printf("ERROR|get ip failed: %v", err)
	}

	for _, address := range addrs {
		// 检查ip地址判断是否回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && !ipnet.IP.IsPrivate() {
			if ipnet.IP.To16() != nil {
				s := ipnet.IP.String()
				arr := strings.Split(s, ":")
				var p string
				for i := 0; i < int(prefix); i++ {
					p = p + arr[i] + ":"
				}
				return p
			}
		}
	}
	return ""
}

func setRecord(record *dnsRecord) bool {
	if record.Value == "" {
		log.Printf("WARN|update alidns no val, ignore: [%s|%s|%s]", record.Type, record.Host, record.Value)
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
		log.Printf("ERROR|update alidns failed: %s, %v", err.Error(), resp)
		return false
	} else {
		log.Printf("INFO|update alidns:%v, [%s|%s|%s]", resp.IsSuccess(), record.Type, record.Host, record.Value)
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
	Locals  []*dnsRecord `json:"locals"`
	Prfixes []*dnsRecord `json:"prfixes"`
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

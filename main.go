package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
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

	ak, err := loadAccessKey()
	if err != nil {
		log.Fatalf("ERROR|loadAccessKey failed: %s", err)
	}
	client, err = alidns.NewClientWithAccessKey("cn-hangzhou", ak.AccessKey, ak.AccessKeySecret)
	if err != nil {
		log.Fatalf("ERROR|open alidns client failed: %s", err)
	}
	defer client.Shutdown()
	log.Printf("INFO|Aliyun ddns started")
	for {
		startDDNS()
		time.Sleep(time.Duration(ak.IntervalMinutes) * time.Minute)
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

func loadRecords() ([]*dnsRecord, error) {
	file, err := os.Open("config/records.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var records []*dnsRecord
	err = dec.Decode(&records)
	if err != nil {
		return nil, err
	}
	return records, nil
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

func startDDNS() {
	records, err := loadRecords()
	if err != nil {
		log.Printf("ERROR|load records failed: %s", err)
		return
	}
	for _, r := range records {
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|getAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			continue
		}
		ip := getIp(r.Type)
		if ip != val {
			r.Value = ip
			setRecord(r)
		}
	}
}

func setRecord(record *dnsRecord) {
	if record.Value == "" {
		log.Printf("WARN|update alidns no val, ignore: [%s|%s|%s]", record.Type, record.Host, record.Value)
		return
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
	} else {
		log.Printf("INFO|update alidns:%v, [%s|%s|%s]", resp.IsSuccess(), record.Type, record.Host, record.Value)
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

type dnsRecord struct {
	RecordId string `json:"recordId"`
	Type     string `json:"type"`
	RR       string `json:"rr"`
	Value    string `json:"value"`
	Host     string `json:"host"`
}
type accessKey struct {
	AccessKey       string `json:"accessKey"`
	AccessKeySecret string `json:"accessKeySecret"`
	IntervalMinutes int32  `json:"intervalMinutes"`
}

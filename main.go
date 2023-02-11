package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
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

	log.Printf("INFO|Starting Aliyun ddns...")
	conf, err := loadConfig()
	if err != nil {
		log.Fatalf("ERROR|Load config failed: %s", err)
	}
	client, err = alidns.NewClientWithAccessKey("cn-hangzhou", conf.AccessKey, conf.AccessKeySecret)
	if err != nil {
		log.Fatalf("ERROR|Open alidns client failed: %s", err)
	}
	defer client.Shutdown()
	log.Printf("INFO|Aliyun ddns is running")

	var startDelay int
	flag.IntVar(&startDelay, "startDelay", 1, "")
	flag.Parse()
	log.Printf("INFO|Start delay(s): %d", startDelay)

	failedCnt := 0
	duration := time.Duration(startDelay) * time.Second
	var curDuration time.Duration
	ticker := time.NewTicker(duration)
	defaultDuration := time.Duration(conf.IntervalMinutes) * time.Minute
	for {
		<-ticker.C
		success := startDDNS(conf)
		duration, failedCnt = nextTick(failedCnt, success, defaultDuration)
		if curDuration != duration {
			ticker.Reset(duration)
			curDuration = duration
		}
	}
}

func nextTick(failedCnt int, success bool, defaultDuration time.Duration) (time.Duration, int) {
	if success {
		return defaultDuration, 0
	} else {
		failedCnt++
		if failedCnt >= 3 {
			log.Printf("ERROR|Failed %d times, do not retry anymore", failedCnt)
			return defaultDuration, 0
		} else {
			log.Printf("ERROR|Failed %d times, retry after 10 seconds", failedCnt)
			return 10 * time.Second, failedCnt
		}
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

func loadConfig() (*config, error) {
	file, err := os.Open("config/config.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var ak config
	err = dec.Decode(&ak)
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

func startDDNS(conf *config) bool {
	pack, err := loadRecords()
	if err != nil {
		log.Printf("ERROR|Load records failed: %s", err)
		return false
	}
	success := true

	var ipv4 string
	var ipv6 string
	if conf.MAC != "" {
		ipv4, ipv6 = findLocalIp(conf.MAC)
	} else {
		ipv4, ipv6 = fetchIp()
	}
	if ipv4 == "" && ipv6 == "" {
		log.Println("ERROR|Could not get ip!")
		return false
	}
	log.Printf("INFO|Fetched ip: [%s|%s]", ipv4, ipv6)

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
	v6ip := net.ParseIP(ipv6)
	if v6ip == nil {
		log.Printf("ERROR|Count not parse ip: %s", ipv6)
		return false
	}
	for _, r := range pack.Prefixes {
		if r.Type != "AAAA" {
			continue
		}
		ip := buildNewIpv6(r, &v6ip)
		if ip == "" {
			log.Printf("ERROR|Find new ip failed: ip=%s, prefix=%d", r.IP, r.Prefix)
			success = false
			continue
		}
		val, err := getAliRecordValue(r.RecordId)
		if err != nil {
			log.Printf("ERROR|GetAliRecordValue failed: %s, recordId=%s", err, r.RecordId)
			success = false
			continue
		}
		if ip != val {
			r.Value = ip
			success = setRecord(r) && success
		}
	}

	return success
}

func buildNewIpv6(r *dnsRecord, ipv6 *net.IP) string {
	oldIp := net.ParseIP(r.IP)
	if oldIp == nil {
		log.Printf("ERROR|Count not parse ip: %s", r.IP)
		return ""
	}
	bytePrefix := (r.Prefix) / 8
	suffix := oldIp[bytePrefix:]
	prefix := (*ipv6)[:bytePrefix]
	return net.IP(append(prefix, suffix...)).String()
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
	IP       string `json:"ip"`
}
type config struct {
	AccessKey       string `json:"accessKey"`
	AccessKeySecret string `json:"accessKeySecret"`
	IntervalMinutes int32  `json:"intervalMinutes"`
	MAC             string `json:"max"`
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

func findLocalIp(mac string) (string, string) {
	hd, err := net.ParseMAC(mac)
	if err != nil {
		log.Printf("ERROR|ParseMAC failed: %v, mac: %s", err, mac)
		return "", ""
	}
	intfs, err := net.Interfaces()
	if err != nil {
		log.Printf("ERROR|Get Interfaces failed: %v", err)
		return "", ""
	}
	for _, intf := range intfs {
		if hd.String() == intf.HardwareAddr.String() {
			addrs, err := intf.Addrs()
			if err != nil {
				log.Printf("ERROR|Get Addrs failed: %v", err)
				return "", ""
			}
			var ipv4 string
			var ipv6 string
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && ipnet.IP.IsGlobalUnicast() {
					if p4 := ipnet.IP.To4(); len(p4) == net.IPv4len {
						ipv4 = ipnet.IP.String()
					} else if ipv6 == "" {
						ipv6 = ipnet.IP.String()
					}
				}
			}
			return ipv4, ipv6
		}
	}
	return "", ""
}

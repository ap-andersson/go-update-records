package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/glesys/glesys-go/v7"
	"github.com/spf13/viper"
)

type DomainRecord struct {
	Host   string
	Domain string
}

type Configurations struct {
	GLESYS_USERNAME       string
	GLESYS_APIKEY         string
	GLESYS_USE_PUBLIC_IP  bool
	GLESYS_IP_STARTS_WITH string
	GLESYS_TTL            int
	GLESYS_INTERVAL       int
	GLESYS_DOMAINS        string
	GLESYS_VERBOSE        bool
}

var verboseLogging bool

func main() {

	log(false, "Starting go-update-records... Woohoo!")

	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log(false, "Error reading config file, %s", err)
		return
	}

	var configuration Configurations

	err := viper.Unmarshal(&configuration)
	if err != nil {
		log(false, "Unable to decode config into struct, %v", err)
		return
	}

	verboseLogging = configuration.GLESYS_VERBOSE

	domains := configuration.GLESYS_DOMAINS

	recordsList := make([]DomainRecord, 0)
	domainSplit := strings.Split(domains, "|")
	for _, domainInfo := range domainSplit {
		domainInfoSplit := strings.Split(domainInfo, "#")
		domain := domainInfoSplit[0]
		hosts := strings.Split(domainInfoSplit[1], ",")
		for _, host := range hosts {
			newRecord := DomainRecord{}
			newRecord.Domain = domain
			newRecord.Host = host
			recordsList = append(recordsList, newRecord)
		}
	}

	for {
		log(true, "Starting run @ %s", time.Now().Format(time.DateTime))

		skippedRecords := make([]string, 0)
		myContext, myCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer myCancel()
		client := glesys.NewClient(configuration.GLESYS_USERNAME, configuration.GLESYS_APIKEY, "go-update-records")

		ipAddress, error := getRelevantIpAddress(configuration)
		if error != nil {
			log(false, "Failed to get relevant IP, skipping run. Reason: %s", error.Error())
			return
		}
		log(true, "Using IP: %s", ipAddress.String())

		var recordsMap = make(map[string][]string)
		for _, record := range recordsList {
			if recordsMap[record.Domain] == nil {
				recordsMap[record.Domain] = make([]string, 0)
			}
			recordsMap[record.Domain] = append(recordsMap[record.Domain], record.Host)
		}

		for domain, hosts := range recordsMap {

			currentRecords, error := client.DNSDomains.ListRecords(myContext, domain)
			if error != nil {
				log(false, "Failed to fetch current records for domain %s. Skipping. Error: %s", domain, error.Error())
				continue
			}

			log(true, "Printing all A records for domain %s", domain)
			for _, r := range *currentRecords {
				if strings.EqualFold(r.Type, "a") {
					log(true, "    %d-%s.%s:%s (%d)",
						r.RecordID,
						r.Host,
						r.DomainName,
						r.Data,
						r.TTL)
				}
			}

			for _, host := range hosts {
				var selectedRecord *glesys.DNSDomainRecord
				for _, r := range *currentRecords {
					if strings.EqualFold(r.Host, host) {
						selectedRecord = &r
						break
					}
				}
				if selectedRecord == nil {
					log(false, "Unable to find host %s, skipping", host)
					continue
				}

				log(true, "Selected record to update: %d-%s.%s:%s (%d)",
					selectedRecord.RecordID,
					selectedRecord.Host,
					selectedRecord.DomainName,
					selectedRecord.Data,
					selectedRecord.TTL)

				if strings.EqualFold(selectedRecord.Data, ipAddress.String()) {
					log(true, "Existing values are correct, no update needed.")
					skippedRecords = append(skippedRecords, fmt.Sprintf("%s.%s", host, domain))
					continue
				}

				log(false, "Updating selected record: %d-%s.%s:%s (%d)",
					selectedRecord.RecordID,
					selectedRecord.Host,
					selectedRecord.DomainName,
					selectedRecord.Data,
					selectedRecord.TTL)

				updateRecordParams := glesys.UpdateRecordParams{}
				updateRecordParams.RecordID = selectedRecord.RecordID
				updateRecordParams.Host = selectedRecord.Host
				updateRecordParams.Data = ipAddress.String()
				updateRecordParams.TTL = selectedRecord.TTL

				if configuration.GLESYS_TTL > 0 {
					updateRecordParams.TTL = configuration.GLESYS_TTL
				}

				updatedRecord, error := client.DNSDomains.UpdateRecord(myContext, updateRecordParams)

				if error != nil {
					log(false, "Failed to update record: %s", error.Error())
					continue
				}

				log(false, "Returned record: %d-%s.%s:%s (%d)",
					updatedRecord.RecordID,
					updatedRecord.Host,
					updatedRecord.DomainName,
					updatedRecord.Data,
					updatedRecord.TTL)
			}

		}

		if len(skippedRecords) > 0 {
			log(false, "Did not need to update: %s", strings.Join(skippedRecords, ", "))
		}

		log(true, "Finishing run @ %s", time.Now().Format(time.DateTime))

		time.Sleep(time.Duration(configuration.GLESYS_INTERVAL) * time.Second)
	}

}

func log(verboseOnly bool, format string, a ...interface{}) {
	if !verboseOnly || verboseLogging {
		if !strings.HasSuffix(format, "\n") {
			format += "\n"
		}
		fmt.Printf(time.Now().Format(time.DateTime)+" "+format, a...)
	}
}

func getRelevantIpAddress(configuration Configurations) (ip net.IP, err error) {
	var returnValue net.IP
	if configuration.GLESYS_USE_PUBLIC_IP {
		log(true, "Finding public IP...")
		resp, error := http.Get("https://api.ipify.org")
		if error != nil {
			log(false, "Error while retrieving the public IP")
			return nil, errors.New("error while fetching the public IP")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log(false, "Error while retrieving the public IP")
			return nil, errors.New("error while reading response when fetching public IP")
		}
		ipAsString := string(body)
		returnValue = net.ParseIP(ipAsString)
		if returnValue == nil {
			return nil, errors.New("failed to parse public IP: " + ipAsString)
		}
		return returnValue, nil
	} else {
		interfaces, err := net.Interfaces()
		if err != nil {
			log(false, "Error while fetching interfaces: %s", err.Error())
			return nil, errors.New("Error while fetching interfaces: " + err.Error())
		}
		for _, currInterface := range interfaces {
			log(true, "  Current interface: %s", currInterface.Name)
			addresses, err := currInterface.Addrs()
			if err != nil {
				log(false, "Error while fetching addresses for interface: %s", err.Error())
				continue
			}
			for _, ipAddress := range addresses {
				log(true, "    Current address: %s", ipAddress.String())
				switch ip := ipAddress.(type) {
				case *net.IPNet:

					if ip.IP.To4() == nil {
						continue
					}

					ipAddressStr := ip.IP.To4().String()
					if strings.HasPrefix(ipAddressStr, configuration.GLESYS_IP_STARTS_WITH) {
						returnValue = net.ParseIP(ipAddressStr)
						return returnValue, nil
					}
				}

			}
		}
	}
	return nil, errors.New("No local IP found that starts with: " + configuration.GLESYS_IP_STARTS_WITH)
}

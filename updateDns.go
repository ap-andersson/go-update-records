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
	GLESYS_WEBSERVICE_URL string
	GLESYS_USERNAME       string
	GLESYS_APIKEY         string
	GLESYS_USE_PUBLIC_IP  bool
	GLESYS_IP_STARTS_WITH string
	GLESYS_TTL            int
	GLESYS_INTERVAL       int
	GLESYS_DOMAINS        string
}

func main() {

	fmt.Println("Starting go-update-records... Woohoo!")

	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s \n", err)
		return
	}

	var configuration Configurations

	err := viper.Unmarshal(&configuration)
	if err != nil {
		fmt.Printf("Unable to decode config into struct, %v \n", err)
		return
	}

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
		fmt.Println("Starting run @ " + time.Now().Format(time.DateTime))

		myContext, myCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer myCancel()
		client := glesys.NewClient(configuration.GLESYS_USERNAME, configuration.GLESYS_APIKEY, "go-update-records")

		ipAddress, error := getRelevantIpAddress(configuration)
		if error != nil {
			fmt.Println("Failed to get relevant IP, skipping run. Reason: ", error.Error())
			return
		}
		fmt.Println("Using IP: " + ipAddress.String())

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
				fmt.Println("Failed to fetch current records for domain " + domain + ". Skipping. Error: " + error.Error())
				continue
			}

			fmt.Println("Printing all A records for domain " + domain)
			for _, r := range *currentRecords {
				if strings.EqualFold(r.Type, "a") {
					fmt.Printf("    %d-%s.%s:%s (%d)\n",
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
					fmt.Printf("Unable to find host %s, skipping \n", host)
					continue
				}
				fmt.Println("Selected record to update:")
				fmt.Printf("    %d-%s.%s:%s (%d)\n",
					selectedRecord.RecordID,
					selectedRecord.Host,
					selectedRecord.DomainName,
					selectedRecord.Data,
					selectedRecord.TTL)

				if strings.EqualFold(selectedRecord.Data, ipAddress.String()) {
					fmt.Println("Existing values are correct, no update needed.")
					continue
				}

				fmt.Println("Updating selected record with new values...")
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
					fmt.Println("Failed to update record: " + error.Error())
					continue
				}

				fmt.Println("Returned record:")
				fmt.Printf("    %d-%s.%s:%s (%d)\n",
					updatedRecord.RecordID,
					updatedRecord.Host,
					updatedRecord.DomainName,
					updatedRecord.Data,
					updatedRecord.TTL)

			}

		}

		fmt.Println("Finishing run @ " + time.Now().Format(time.DateTime))

		time.Sleep(time.Duration(configuration.GLESYS_INTERVAL) * time.Second)
	}

}

func getRelevantIpAddress(configuration Configurations) (ip net.IP, err error) {
	var returnValue net.IP
	if configuration.GLESYS_USE_PUBLIC_IP {
		fmt.Println("Finding public IP...")
		resp, error := http.Get("https://api.ipify.org")
		if error != nil {
			fmt.Println("Error while retrieving the public IP")
			return nil, errors.New("error while fetching the public IP")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error while retrieving the public IP")
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
			fmt.Print(fmt.Errorf("Error while fetching interfaces: " + err.Error()))
			return nil, errors.New("Error while fetching interfaces: " + err.Error())
		}
		for _, currInterface := range interfaces {
			fmt.Println("    Current interface: " + currInterface.Name)
			addresses, err := currInterface.Addrs()
			if err != nil {
				fmt.Print(fmt.Errorf("Error while fetching addresses for interface: " + err.Error()))
				continue
			}
			for _, ipAddress := range addresses {
				fmt.Println("    Current address: " + ipAddress.String())
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

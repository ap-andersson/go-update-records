# go-update-records
A GO application packaged as a docker container that uses the GO library provided by hosting provider Glesys to update DNS records with new IP address.

## Introduction

I am writing this application only to fulfill my needs, which is to either update records on domains with my public IP or any IP found on any adapter that starts with a given string. The first version was written in C# but since Glesys provides a GO library this was he perfect project to try-out the GO language. This is my first real GO project.

This project is useful to keep your domain synced with your public IP if you don't have static IP where you host services, in your homelab for example.

The option to find an IP address based on a string is useful if you want to use your domain name internally in a network, where you want the application to find your current local network IP (when you are using DHCP) and update the domain with that.

You only need listrecords and updaterecord permission on the account used to access Glesys API at the moment. Which means that the application cannot create or delete records. You pre-create any records you want to be updated.

## Environemtn variables

| Variable | Required | Default value | Description
| --- | ----------- | --- | ---
| GLESYS_USE_PUBLIC_IP | Yes |  | Whether to use public IP or IP found by using GLESYS_IP_STARTS_WITH variable
| GLESYS_TTL | No | Existing value on record | TTL to set on the records
| GLESYS_INTERVAL | No | 300 | Interval to run in seconds
| GLESYS_IP_STARTS_WITH | No | | String used to find IP on any adapter to use for the updates. Used together with USE_PUBLIC_IP = false
| GLESYS_USERNAME | Yes | | Username (starts with 'cl') for the API
| GLESYS_APIKEY | Yes | | API key for the account
| GLESYS_DOMAINS | Yes | | Domains and hosts that should be updated. Format is '\<domain1>#\<host1>,\<host2>\|\<domain2>#\<host1>,\<host2>'
| GLESYS_VERBOSE | No | false | Enable verbose logging


## Docker container
There is a container that is built every time code is pushed and automatically published. The path to the image is ghcr.io/ap-andersson/go-update-records:master. Below is a docker-compose template.

When you want to use your public IP address you do not need to use netowrk_mode as host.

``` 
services:
  dns-updater-go:
    image: ghcr.io/ap-andersson/go-update-records:master
    container_name: dns-updater-go
    environment:
      - GLESYS_USERNAME=clxxxx
      - GLESYS_APIKEY=
      - GLESYS_USE_PUBLIC_IP=true
      #- GLESYS_IP_STARTS_WITH=192.168.0.
      - GLESYS_DOMAINS=example1.com#@,www,*|example1.com#@,www,*
      #- GLESYS_VERBOSE=true
    #network_mode: "host"
    restart: unless-stopped
```

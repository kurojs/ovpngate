package vpngate

import (
	"encoding/base64"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
)

func Fetch() ([]Server, error) {
	resp, err := http.Get("https://www.vpngate.net/api/iphone/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.Comment = '*'
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var servers []Server

	for _, row := range records[1:] {
		if len(row) < 15 {
			continue
		}

		ping, _ := strconv.Atoi(row[3])
		speed, _ := strconv.Atoi(row[4])
		sessions, _ := strconv.Atoi(row[7])

		ovpnRaw := strings.TrimSpace(row[14])
		ovpnBytes, err := base64.StdEncoding.DecodeString(ovpnRaw)
		if err != nil {
			continue
		}

		servers = append(servers, Server{
			HostName:     row[0],
			IP:           row[1],
			Ping:         ping,
			Speed:        speed / 125000,
			CountryLong:  row[5],
			CountryShort: row[6],
			Sessions:     sessions,
			OvpnConfig:   ovpnBytes,
		})
	}

	return servers, nil
}

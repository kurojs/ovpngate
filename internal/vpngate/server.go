package vpngate

type Server struct {
	HostName    string
	IP          string
	Ping        int
	Speed       int
	CountryLong string
	CountryShort string
	Sessions    int
	OvpnConfig  []byte
}

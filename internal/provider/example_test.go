package provider

import (
	"fmt"
	"net/netip"
)

func ExampleBuildSingleAddressPlan() {
	ipv4 := netip.MustParseAddr("198.51.100.10")

	plan, err := BuildSingleAddressPlan(
		State{Name: "host.example.com."},
		DesiredState{
			Name:       "host.example.com.",
			TTLSeconds: 300,
			IPv4:       &ipv4,
		},
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(plan.Summaries()[0])
	// Output:
	// create A host.example.com. -> 198.51.100.10 ttl=300
}

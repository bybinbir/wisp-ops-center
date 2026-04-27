package mimosa

// client.go is intentionally tiny. Phase 4 only wires SNMP read-only
// access; SNMPClient sits in snmp_client.go and the orchestrator
// (internal/devicectl) calls Probe/Poll directly. A future Phase 5
// vendor API client would land here.

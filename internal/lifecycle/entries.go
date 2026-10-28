package lifecycle

// Entry is a GasaCLI-owned path that lifecycle commands may inspect or remove.
type Entry struct {
	Name   string
	All    bool
	Backup bool
}

// Entries is the single closed list of paths managed by GasaCLI.
var Entries = []Entry{
	{Name: "config.yaml"},
	{Name: "state.json"},
	{Name: "trust.json"},
	{Name: "trust.json.lock"},
	{Name: "logs/"},
	{Name: "agents/", All: true},
	{Name: "prompts/", All: true},
	{Name: "skills/", All: true},
	{Name: "plugins/", All: true},
	{Name: "segundo-cerebro/", All: true},
	{Name: "gasacli.db", All: true},
	{Name: "gasacli.db-wal", All: true},
	{Name: "gasacli.db-shm", All: true},
	{Name: "backups/", Backup: true},
	{Name: ".gasacli", All: true},
}

func IsDirectoryEntry(name string) bool { return len(name) > 0 && name[len(name)-1] == '/' }

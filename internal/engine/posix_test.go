package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPOSIXResourceAndDynamicLookupEvidence(t *testing.T) {
	root := t.TempDir()
	source := `#define CHANNEL "/demo-memory"
void implementation(void){}
int main(){
 int fd=socket(AF_UNIX,SOCK_DGRAM,0);
 struct sockaddr_un address;
 strcpy(address.sun_path,"/tmp/demo.sock");
 bind(fd,(struct sockaddr*)&address,sizeof(address));
 sendto(fd,"x",1,0,(struct sockaddr*)&address,sizeof(address));
 int memory=shm_open(CHANNEL,O_CREAT|O_RDWR,0666);
 mmap(0,4096,PROT_READ,MAP_SHARED,memory,0);
 dlsym(RTLD_NEXT,"implementation");
 recvfrom(fd,0,0,0,0,0);
}
`
	if err := os.WriteFile(filepath.Join(root, "main.c"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := scanSources(context.Background(), root, false, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	posix := facts.posix.Facts(&facts.macros)
	if len(posix.UnixSocket.Endpoints) != 2 {
		t.Fatalf("socket: %+v", posix.UnixSocket)
	}
	for _, endpoint := range posix.UnixSocket.Endpoints {
		if endpoint.Name != "/tmp/demo.sock" || endpoint.Transport != "datagram" || len(endpoint.Entrypoints) != 1 {
			t.Fatal(endpoint)
		}
	}
	if len(posix.SharedMemory.Endpoints) != 1 || posix.SharedMemory.Endpoints[0].Name != "/demo-memory" {
		t.Fatal(posix.SharedMemory)
	}
	if len(facts.graph.DynamicBindings) != 1 || len(facts.graph.DynamicBindings[0].CandidateSymbolIDs) != 1 || facts.graph.DynamicBindings[0].Status != "resolved-name" {
		t.Fatal(facts.graph.DynamicBindings)
	}
	foundUnknown := false
	for _, observation := range posix.UnixSocket.Observations {
		if observation.API == "recvfrom" && observation.Status == "unresolved" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal("unresolved native operations were hidden")
	}
}
func TestPOSIXDoesNotInventResourcesFromIndirectGlobalFields(t *testing.T) {
	root := t.TempDir()
	source := `void transport(struct data *p){int fd=socket(AF_UNIX,SOCK_DGRAM,0);strcpy(address.sun_path,p->path);bind(fd,(struct sockaddr*)&address,sizeof(address));}`
	if err := os.WriteFile(filepath.Join(root, "native.c"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := scanSources(context.Background(), root, false, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	result := facts.posix.Facts(&facts.macros)
	if len(result.UnixSocket.Endpoints) != 0 || len(result.UnixSocket.Observations) != 2 {
		t.Fatal(result)
	}
}

func TestPOSIXReusesBoundedNamedEntrypointTrace(t *testing.T) {
	root := t.TempDir()
	source := `void create_resource(){shm_open("/demo",O_CREAT|O_RDWR,0666);} int main(){create_resource();}`
	if err := os.WriteFile(filepath.Join(root, "source.c"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := scanSources(context.Background(), root, false, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	result := facts.posix.Facts(&facts.macros)
	facts.ipc.AttachPOSIXEntrypoints(result)
	entries := result.SharedMemory.Endpoints[0].Entrypoints
	if len(entries) != 1 || entries[0].Kind != "main" || len(entries[0].CallChain) != 1 {
		t.Fatal(entries)
	}
}

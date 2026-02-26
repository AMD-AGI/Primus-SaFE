Currently the following monitors are supported:

| Category | Monitor | Error Code|
|-----------|---------|---------|
| GPU       | Check existance of amdgpu module and rocm-smi | 001 |
|           | Check ECC and PCIe replay errors | 002 |
|           | Check bad-pages | 003 |
|           | Check GPU count | 004 |
|           | Check GPU device plug-in * | 005 |
|           | Check GPU temperature | 006 |
|           | Check GPU Driver version | 007 |
|           | Monitor RAS enablement and error| 008 |
|           | Check cross GPU linkage | 009 |
|           | Monitor xgmi link error | 010 |
|           | Check ROCm version| 011 |
|           | Monitor GPU IPC error | 012 |
|           | Check rocminfo existance| 013 |
| Network   | Check IB status | 201 |
|           | Run network device test | 202 |
|           | Check network interface | 203 |
|           | Check Bnxt modules | 204 |
|           | Check AINIC modules| 205 |
|           | Check userspace RDMA devices| 206 |
|           | Check RDMA GID and attemp to fix| 207 |
|           | Check AINIC device count | 208 |
|           | Check DNS service status| 209 |
|System     | Verify UTC timezone | 301 |
|           | Check hostname | 302 |
|           | Check containerd service status| 303 |
|           | Monitor Kernel lockup in system log| 304 |
|           | Check PCI Access Control Services| 305 |
|           | Check Boot arguments| 306 |
|           | | 307 |
|           | Monitor PCIe error | 308 |
|           | Monitor Wekafs CSI status * | 309 |
|File System| Check NFS mount  | 401 |



*Kubernetes only

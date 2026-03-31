# Internet Bandwidth Benchmark & Packet Loss Test

The scripts include both an Internet download speed test as well as a packet loss analysis. To enable these tests, set the environment variable ```RUN_INTERNET_BANDWIDTH_BENCHMARK``` and ```RUN_PACKET_LOSS_TEST``` to ```true``` respectively.

By default, they are using http://speedtest.newark.linode.com/100MB-newark.bin as the download target, which is controlled by environment varaible ```SPEEDTEST_URL```. If this needs to be changed, modify the ```SPEEDTEST_TARGET_IP``` environment variable as well that being used by tcpdump for packet loss analysis.

Other settings include ```SPEEDTEST_THREADS_PER_NODE``` and ```SPEEDTEST_ROUNDS``` control the parallelism and rounds of the download.

For the output, the speed test will generate an average download speed during the test, while the packet loss test generates the total number of packet losses during the test.
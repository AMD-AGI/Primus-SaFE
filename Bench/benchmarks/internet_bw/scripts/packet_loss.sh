set -Eeuo pipefail

echo "Packet loss test: $SPEEDTEST_URL"
echo "---"

tcpdump -i any -w /tmp/tcpdump.pcap host $SPEEDTEST_TARGET_IP &
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/download.sh"; pkill -INT tcpdump

packet_loss=$(tshark -r /tmp/tcpdump.pcap -Y 'tcp.analysis.retransmission' -t ad 2>/dev/null | wc -l)   
echo "Packet loss: $packet_loss"

rm -f /tmp/tcpdump.pcap

exit $packet_loss
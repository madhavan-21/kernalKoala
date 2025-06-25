eBPF Network Monitor-Kernel koala

![Custom Image](/config/assest/kernalkoala.png)


This project is a minimal eBPF-based network monitor that uses tc (Traffic Control) hooks to trace ingress and egress network traffic in real time. It extracts key metadata like IP addresses, ports, protocol types, and TCP flags from packets and sends this metadata to user space via perf events.


📦 Features

1 . Hooks into both ingress and egress traffic using tc.

2 . Parses Ethernet, IP, TCP, and UDP headers.

3 . Emits structured events to user space with:

     -Source & destination IP

     -Source & destination port

     -Protocol

     -TCP flags

     -Direction (ingress/egress)

🧠 How It Works

eBPF Program
   1.Attached to network interfaces using tc (clsact qdisc).
   
   2.process_packet():

     -> Verifies Ethernet and IP headers.
     -> Parses TCP or UDP headers.
     -> Builds an event structure.
     -> Emits the event using bpf_perf_event_output.

Data Path
```css
     [ NIC ] --> [ tc_ingress() ] --> [ eBPF perf map ] --> [ user-space reader ]
                            \--> [ tc_egress() ]
```

tc_ingress() is called on incoming packets.
tc_egress() is called on outgoing packets.
Events are collected using a perf buffer in your Go/Python/C user space program.


```bash
  Follow DevReadme.md file to Build, Test, and Use the eBPF Network Monitor.
```

📄 License
This project uses GPL-2.0 license to comply with kernel BPF requirements.



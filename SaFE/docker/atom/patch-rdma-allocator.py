#!/usr/bin/env python3
"""Point the RDMA protocol at the HIP allocator and default to dma-buf."""

from pathlib import Path

root = Path("/sgl-workspace/Mooncake")

py = root / "mooncake-integration/transfer_engine/transfer_engine_py.cpp"
text = py.read_text()
old = """    } else {
        allocateMemory = malloc;
        freeMemory = free;
        LOG(WARNING) << "Using default malloc/free for protocol: " << protocol;
    }
"""
new = """    } else if (strcmp(protocol, "rdma") == 0) {
#ifdef USE_HIP
        // GPU memory for the RDMA protocol. Host malloc cannot be registered
        // with the NIC through dma-buf.
        allocateMemory = [](size_t s) -> void* {
            return mooncake::HipTransport::allocatePinnedLocalMemory(s);
        };
        freeMemory = [](void* p) {
            mooncake::HipTransport::freePinnedLocalMemory(p);
        };
        LOG(INFO) << "Selected HIP memory allocator for protocol: " << protocol;
#else
        allocateMemory = malloc;
        freeMemory = free;
        LOG(WARNING) << "Using default malloc/free for protocol: " << protocol;
#endif
    } else {
        allocateMemory = malloc;
        freeMemory = free;
        LOG(WARNING) << "Using default malloc/free for protocol: " << protocol;
    }
"""
if "Selected HIP memory allocator for protocol" not in text:
    if old not in text:
        raise SystemExit("allocator anchor missing")
    py.write_text(text.replace(old, new, 1))

ctx = root / "mooncake-transfer-engine/src/transport/rdma_transport/rdma_context.cpp"
ct = ctx.read_text()
marker = "Peer memory is selected only when"
if marker not in ct:
    start = ct.find("bool isKernelDmabufSupported() {")
    end = ct.find("#endif  // USE_HIP_DMABUF", start if start >= 0 else 0)
    if start < 0 or end < 0:
        raise SystemExit("dmabuf probe anchor missing")
    replacement = """bool isKernelDmabufSupported() {
    // Default is dma-buf. Peer memory is selected only when
    // MOONCAKE_DISABLE_HIP_DMABUF is set to a value other than 0.
    static const bool supported = []() {
        const char *env = std::getenv("MOONCAKE_DISABLE_HIP_DMABUF");
        if (env != nullptr && std::string(env) != "0") {
            LOG(INFO) << "HIP dmabuf disabled via MOONCAKE_DISABLE_HIP_DMABUF";
            return false;
        }
        return true;
    }();
    return supported;
}

"""
    ctx.write_text(ct[:start] + replacement + ct[end:])
    ct = ctx.read_text()

marker_export = "dmabuf export keeps the caller HIP context"
marker_off_thread = "dmabuf export runs off the caller HIP context"
marker_restore = "dmabuf export restores the caller HIP context"
off_thread_tail = """    // HIP queries and the dma-buf export run on a joined side thread.
    // Retain the caller's primary context across that work and put it
    // back afterwards. Without the retain, the next hipStreamCreate
    // on this thread segfaults.
    hipCtx_t caller_ctx = nullptr;
    (void)hipCtxGetCurrent(&caller_ctx);
    hipCtx_t retained_ctx = nullptr;
    (void)hipDevicePrimaryCtxRetain(&retained_ctx, hipAttr.device);
    int rc = ERR_CONTEXT;
    std::thread([&]() {
        if (hipSetDevice(hipAttr.device) != hipSuccess) {
            LOG(ERROR) << "Failed to set HIP device to " << hipAttr.device
                       << " for dmabuf export of " << (uintptr_t)addr;
            return;
        }
        rc = exportCurrentDeviceDmabuf(addr, out);
    }).join();
    if (caller_ctx != nullptr) {
        (void)hipCtxSetCurrent(caller_ctx);
    }
    if (rc == 0) {
        LOG_FIRST_N(INFO, 1) << "dmabuf export restores the caller HIP context";
    }
    return rc;
}"""
previous_off_thread_tail = """    // HIP queries and the dma-buf export run on a joined side thread.
    // Doing them on the scheduler thread leaves PyTorch's HIP context
    // unusable: the next hipStreamCreate segfaults.
    int rc = ERR_CONTEXT;
    std::thread([&]() {
        if (hipSetDevice(hipAttr.device) != hipSuccess) {
            LOG(ERROR) << "Failed to set HIP device to " << hipAttr.device
                       << " for dmabuf export of " << (uintptr_t)addr;
            return;
        }
        rc = exportCurrentDeviceDmabuf(addr, out);
    }).join();
    if (rc == 0) {
        LOG_FIRST_N(INFO, 1) << "dmabuf export runs off the caller HIP context";
    }
    return rc;
}"""
if marker_restore in ct:
    pass
elif marker_off_thread in ct:
    if previous_off_thread_tail not in ct:
        raise SystemExit("off-thread export anchor missing")
    ctx.write_text(ct.replace(previous_off_thread_tail, off_thread_tail, 1))
elif marker_export in ct:
    same_thread_tail = """    // The scheduler thread already holds the PyTorch HIP context.
    // hipSetDevice on that thread replaces the context, and the next
    // hipStreamCreate segfaults. Export here when the device matches.
    // A different device is switched only on a joined side thread.
    int current = -1;
    if (hipGetDevice(&current) == hipSuccess && current == hipAttr.device) {
        LOG_FIRST_N(INFO, 1) << "dmabuf export keeps the caller HIP context";
        return exportCurrentDeviceDmabuf(addr, out);
    }
    int rc = ERR_CONTEXT;
    std::thread([&]() {
        if (hipSetDevice(hipAttr.device) != hipSuccess) {
            LOG(ERROR) << "Failed to set HIP device to " << hipAttr.device
                       << " for dmabuf export of " << (uintptr_t)addr;
            return;
        }
        rc = exportCurrentDeviceDmabuf(addr, out);
    }).join();
    return rc;
}"""
    if same_thread_tail not in ct:
        raise SystemExit("same-thread export anchor missing")
    ctx.write_text(ct.replace(same_thread_tail, off_thread_tail, 1))
elif marker_export not in ct:
    if "#include <thread>" not in ct:
        inc = ct.find("#include ")
        if inc < 0:
            raise SystemExit("rdma_context include anchor missing")
        line_end = ct.find("\n", inc)
        ct = ct[: line_end + 1] + "#include <thread>\n" + ct[line_end + 1 :]
    old_export = """    HipDeviceGuard dev_guard(hipAttr.device);
    if (!dev_guard.set_ok()) {
        LOG(ERROR) << "Failed to set HIP device to " << hipAttr.device
                   << " for dmabuf export of " << (uintptr_t)addr;
        return ERR_CONTEXT;
    }
    hipDeviceptr_t allocBase = nullptr;
    size_t allocSize = 0;
    hipRes = hipMemGetAddressRange(&allocBase, &allocSize,
                                   reinterpret_cast<hipDeviceptr_t>(addr));
    if (hipRes != hipSuccess) {
        LOG(ERROR) << "Failed to call hipMemGetAddressRange for "
                   << (uintptr_t)addr
                   << " hip error=" << hipGetErrorString(hipRes);
        return ERR_CONTEXT;
    }
    int dmabuf_fd = -1;
    uint64_t hsa_dmabuf_offset = 0;
    hsa_status_t hsaRes = hsa_amd_portable_export_dmabuf(
        allocBase, allocSize, &dmabuf_fd, &hsa_dmabuf_offset);
    if (hsaRes != HSA_STATUS_SUCCESS) {
        const char *hsaErr = nullptr;
        hsa_status_string(hsaRes, &hsaErr);
        LOG(ERROR) << "Failed to retrieve dmabuf for " << (uintptr_t)addr
                   << " base=" << (uintptr_t)allocBase << " size=" << allocSize
                   << " hsa error=" << (hsaErr ? hsaErr : "unknown");
        return ERR_CONTEXT;
    }
    out.use_dmabuf = true;
    out.fd = dmabuf_fd;
    out.offset = (uintptr_t)addr - (uintptr_t)allocBase + hsa_dmabuf_offset;
    return 0;
}"""
    new_export = off_thread_tail
    helper = """int exportCurrentDeviceDmabuf(void *addr, HipDmabufExport &out) {
    hipDeviceptr_t allocBase = nullptr;
    size_t allocSize = 0;
    hipError_t hipRes = hipMemGetAddressRange(
        &allocBase, &allocSize, reinterpret_cast<hipDeviceptr_t>(addr));
    if (hipRes != hipSuccess) {
        LOG(ERROR) << "Failed to call hipMemGetAddressRange for "
                   << (uintptr_t)addr
                   << " hip error=" << hipGetErrorString(hipRes);
        return ERR_CONTEXT;
    }
    int dmabuf_fd = -1;
    uint64_t hsa_dmabuf_offset = 0;
    hsa_status_t hsaRes = hsa_amd_portable_export_dmabuf(
        allocBase, allocSize, &dmabuf_fd, &hsa_dmabuf_offset);
    if (hsaRes != HSA_STATUS_SUCCESS) {
        const char *hsaErr = nullptr;
        hsa_status_string(hsaRes, &hsaErr);
        LOG(ERROR) << "Failed to retrieve dmabuf for " << (uintptr_t)addr
                   << " base=" << (uintptr_t)allocBase << " size=" << allocSize
                   << " hsa error=" << (hsaErr ? hsaErr : "unknown");
        return ERR_CONTEXT;
    }
    out.use_dmabuf = true;
    out.fd = dmabuf_fd;
    out.offset = (uintptr_t)addr - (uintptr_t)allocBase + hsa_dmabuf_offset;
    return 0;
}

"""
    if old_export not in ct:
        raise SystemExit("exportHipDmabuf anchor missing")
    fn = ct.find("int exportHipDmabuf(void *addr, HipDmabufExport &out) {")
    if fn < 0:
        raise SystemExit("exportHipDmabuf function missing")
    ct = ct[:fn] + helper + ct[fn:]
    ct = ct.replace(old_export, new_export, 1)
    ctx.write_text(ct)

print("rdma allocator patched")

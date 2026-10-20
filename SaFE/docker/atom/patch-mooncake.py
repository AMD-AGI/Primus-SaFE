#!/usr/bin/env python3
from pathlib import Path

root = Path("/sgl-workspace/Mooncake")

# Drop HIP below RDMA when MC_DISABLE_HIP is set. Cross-node transfers then
# stay on RDMA instead of HIP IPC.
multi = root / "mooncake-transfer-engine/src/multi_transport.cpp"
mt = multi.read_text()
if "#include <cstdlib>" not in mt:
    anchor = "#include <string>\n"
    if anchor not in mt:
        raise SystemExit("cstdlib include anchor missing")
    mt = mt.replace(anchor, anchor + "#include <cstdlib>\n", 1)
old = '            if (p == "hip") return 4;\n'
new = '            if (p == "hip") return std::getenv("MC_DISABLE_HIP") ? 0 : 4;\n'
if old not in mt:
    raise SystemExit("hip priority anchor missing")
multi.write_text(mt.replace(old, new, 1))

# rdma_context.cpp is compiled into rdma_transport. The macro must be set on
# that target, or the installed engine.so omits the dmabuf path.
cmake = root / "mooncake-transfer-engine/src/CMakeLists.txt"
cm = cmake.read_text()
old = """      target_compile_definitions(transfer_engine PRIVATE USE_HIP_DMABUF)
      target_link_libraries(transfer_engine PUBLIC hsa-runtime64::hsa-runtime64)
"""
new = """      target_compile_definitions(transfer_engine PRIVATE USE_HIP_DMABUF)
      target_link_libraries(transfer_engine PUBLIC hsa-runtime64::hsa-runtime64)
      target_compile_definitions(rdma_transport PRIVATE USE_HIP_DMABUF)
      target_link_libraries(rdma_transport PUBLIC hsa-runtime64::hsa-runtime64)
"""
if old not in cm:
    raise SystemExit("cmake dmabuf anchor missing")
cmake.write_text(cm.replace(old, new, 1))

guard = root / "mooncake-transfer-engine/include/hip_device_guard.h"
gt = guard.read_text()
old = """    explicit HipDeviceGuard(int target_device) : HipDeviceGuard() {
        set_ok_ = (hipSetDevice(target_device) == hipSuccess);
    }
"""
new = """    // hipSetDevice only after hipGetDevice succeeds. The destructor restores
    // the previous device only when that query succeeded.
    explicit HipDeviceGuard(int target_device) : HipDeviceGuard() {
        if (!saved_) {
            set_ok_ = false;
            return;
        }
        set_ok_ = (hipSetDevice(target_device) == hipSuccess);
    }
"""
if old not in gt:
    raise SystemExit("guard anchor missing")
guard.write_text(gt.replace(old, new, 1))

ctx_h = root / "mooncake-transfer-engine/include/transport/rdma_transport/rdma_context.h"
ht = ctx_h.read_text()
old = "    int registerMemoryRegion(void *addr, size_t length, int access);\n"
new = """    struct SharedDmabuf {
        bool use_dmabuf = false;
        int fd = -1;
        uint64_t base_offset = 0;
    };

    // Export one dma-buf for the allocation that contains addr.
    int exportSharedDmabuf(void *addr, SharedDmabuf &out);
    static void closeSharedDmabuf(SharedDmabuf &exp);
    // Register one slice. extra_offset is added to the exported base offset.
    // This does not call HIP.
    int registerMemoryRegion(void *addr, size_t length, int access,
                             const SharedDmabuf &exp, uint64_t extra_offset);

    int registerMemoryRegion(void *addr, size_t length, int access);
"""
if old not in ht:
    raise SystemExit("context header anchor missing")
ctx_h.write_text(ht.replace(old, new, 1))

tr_h = root / "mooncake-transfer-engine/include/transport/rdma_transport/rdma_transport.h"
tt = tr_h.read_text()
old = "    std::vector<std::shared_ptr<RdmaContext>> context_list_;\n"
new = """    std::vector<std::shared_ptr<RdmaContext>> context_list_;
    std::mutex chunk_map_mutex_;
    std::unordered_map<uint64_t, std::vector<uint64_t>> chunk_map_;
"""
if old not in tt:
    raise SystemExit("transport header anchor missing")
tr_h.write_text(tt.replace(old, new, 1))

ctx = root / "mooncake-transfer-engine/src/transport/rdma_transport/rdma_context.cpp"
ct = ctx.read_text()
old = """    if (length > (size_t)globalConfig().max_mr_size) {
        PLOG(WARNING) << "The buffer length exceeds device max_mr_size, "
                      << "shrink it to " << globalConfig().max_mr_size;
        length = (size_t)globalConfig().max_mr_size;
    }
"""
new = """    if (length > (size_t)globalConfig().max_mr_size) {
        LOG(ERROR) << "Buffer length " << length
                   << " exceeds device max_mr_size "
                   << globalConfig().max_mr_size;
        return ERR_INVALID_ARGUMENT;
    }
"""
if old not in ct:
    raise SystemExit("shrink anchor missing")
ct = ct.replace(old, new, 1)

start = ct.find("#elif defined(USE_HIP_DMABUF)\n")
if start < 0:
    raise SystemExit("hip branch missing")
else_at = ct.find("\n#else\n", start)
if else_at < 0:
    raise SystemExit("hip branch else missing")
hip_replacement = r'''#elif defined(USE_HIP_DMABUF)
    HipDmabufExport hip_exp;
    int hip_ret = exportHipDmabuf(addr, hip_exp);
    if (hip_ret != 0) {
        return hip_ret;
    }
    mrMeta.addr = addr;
    if (hip_exp.use_dmabuf) {
        mrMeta.mr = ibv_reg_dmabuf_mr(pd_, hip_exp.offset, length,
                                      (uintptr_t)addr, hip_exp.fd, access);
        const int regErrno = errno;
        if (close(hip_exp.fd) != 0) {
            PLOG(WARNING) << "Failed to close dmabuf fd";
        }
        hip_exp.fd = -1;
        if (!mrMeta.mr) {
            errno = regErrno;
        }
    } else {
        mrMeta.mr = ibv_reg_mr(pd_, addr, length, access);
    }
'''
ct = ct[:start] + hip_replacement + ct[else_at + 1:]

helper = r'''
namespace {

std::mutex &hipExportMutex() {
    static std::mutex mu;
    return mu;
}

struct HipDmabufExport {
    bool use_dmabuf = false;
    int fd = -1;
    uint64_t offset = 0;
};

#if defined(USE_HIP_DMABUF)
int exportHipDmabuf(void *addr, HipDmabufExport &out) {
    std::lock_guard<std::mutex> hip_export_lock(hipExportMutex());
    hipPointerAttribute_t hipAttr{};
    hipError_t hipRes = hipPointerGetAttributes(&hipAttr, addr);
    out = HipDmabufExport{};
    if (hipRes != hipSuccess || hipAttr.type == hipMemoryTypeHost ||
        hipAttr.type == hipMemoryTypeUnregistered) {
        return 0;
    }
    if (hipAttr.type == hipMemoryTypeManaged) {
        LOG(WARNING) << "HIP managed memory at " << (uintptr_t)addr
                     << " — dmabuf export skipped (pages may migrate); "
                        "falling back to ibv_reg_mr";
        return 0;
    }
    if (hipAttr.type == hipMemoryTypeDevice && !isKernelDmabufSupported()) {
        return 0;
    }
    if (hipAttr.type != hipMemoryTypeDevice) {
        return 0;
    }
    HipDeviceGuard dev_guard(hipAttr.device);
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
}
#endif

}  // namespace

'''
anchor = "int RdmaContext::registerMemoryRegionInternal"
pos = ct.find(anchor)
if pos < 0:
    raise SystemExit("registerMemoryRegionInternal missing")
ct = ct[:pos] + helper + ct[pos:]

methods = r'''
int RdmaContext::exportSharedDmabuf(void *addr, SharedDmabuf &out) {
    out = SharedDmabuf{};
#if defined(USE_HIP_DMABUF)
    HipDmabufExport exp;
    int ret = exportHipDmabuf(addr, exp);
    if (ret != 0) {
        return ret;
    }
    out.use_dmabuf = exp.use_dmabuf;
    out.fd = exp.fd;
    out.base_offset = exp.offset;
#else
    (void)addr;
#endif
    return 0;
}

void RdmaContext::closeSharedDmabuf(SharedDmabuf &exp) {
    if (exp.fd >= 0) {
        if (close(exp.fd) != 0) {
            PLOG(WARNING) << "Failed to close dmabuf fd";
        }
        exp.fd = -1;
    }
}

int RdmaContext::registerMemoryRegion(void *addr, size_t length, int access,
                                      const SharedDmabuf &exp,
                                      uint64_t extra_offset) {
    if (length > (size_t)globalConfig().max_mr_size) {
        LOG(ERROR) << "Buffer length " << length
                   << " exceeds device max_mr_size "
                   << globalConfig().max_mr_size;
        return ERR_INVALID_ARGUMENT;
    }
    MemoryRegionMeta mrMeta;
    mrMeta.addr = addr;
    if (exp.use_dmabuf) {
        mrMeta.mr = ibv_reg_dmabuf_mr(pd_, exp.base_offset + extra_offset, length,
                                      (uintptr_t)addr, exp.fd, access);
    } else {
        mrMeta.mr = ibv_reg_mr(pd_, addr, length, access);
    }
    if (!mrMeta.mr) {
        PLOG(ERROR) << "Failed to register memory " << addr;
        return ERR_CONTEXT;
    }
    RWSpinlock::WriteGuard guard(memory_regions_lock_);
    memory_region_map_[reinterpret_cast<uintptr_t>(mrMeta.addr)] = mrMeta;
    return 0;
}

'''
# exportHipDmabuf is in an anonymous namespace, so the method below it in the
# same translation unit can call it only if the method is placed after the
# helper. Insert methods after the helper, before registerMemoryRegionInternal.
ct = ct.replace(helper + anchor, helper + methods + anchor, 1)
ctx.write_text(ct)

transport = root / "mooncake-transfer-engine/src/transport/rdma_transport/rdma_transport.cpp"
tt = transport.read_text()
old_pre_start = tt.find("int RdmaTransport::preTouchMemory")
old_pre_end = tt.find("int RdmaTransport::registerLocalMemory(")
if old_pre_start < 0 or old_pre_end < 0:
    raise SystemExit("preTouch anchors missing")
tt = tt[:old_pre_start] + """int RdmaTransport::preTouchMemory(void *addr, size_t length) {
    if (context_list_.empty() || length == 0) {
        return 0;
    }
    // One call. Threads here would export dma-bufs through HIP concurrently.
    return context_list_[0]->preTouchMemory(addr, length);
}

""" + tt[old_pre_end:]

start = tt.find("    bool do_pre_touch = context_list_.size() > 0 &&")
end = tt.find("int RdmaTransport::unregisterLocalMemory")
if start < 0 or end < 0:
    raise SystemExit("register body anchors missing")
new_body = r'''    const bool do_pre_touch = !context_list_.empty() &&
                              std::thread::hardware_concurrency() >= 4 &&
                              length >= (size_t)4 * 1024 * 1024 * 1024;
    size_t chunk_limit = (size_t)globalConfig().max_mr_size;
    std::vector<std::pair<void *, size_t>> chunks;
    if (chunk_limit > 0 && length > chunk_limit) {
        for (size_t offset = 0; offset < length;) {
            size_t chunk_len = std::min(chunk_limit, length - offset);
            chunks.emplace_back(static_cast<char *>(addr) + offset, chunk_len);
            offset += chunk_len;
        }
        LOG(WARNING) << "Auto-splitting buffer " << addr << " (" << length
                     << " bytes) into " << chunks.size()
                     << " chunks of <= " << chunk_limit << " bytes";
    } else {
        chunks.emplace_back(addr, length);
    }

    std::string resolved_name;
    if (name == kWildcardLocation) {
        bool only_first_page = true;
        const std::vector<MemoryLocationEntry> entries =
            getMemoryLocation(addr, length, only_first_page);
        if (entries.empty()) return -1;
        resolved_name = entries[0].location;
    } else {
        resolved_name = name;
    }

    RdmaContext::SharedDmabuf shared;
    bool exported = false;
    if (!context_list_.empty()) {
        int eret = context_list_[0]->exportSharedDmabuf(addr, shared);
        if (eret != 0) {
            LOG(ERROR) << "Failed to export dma_buf for addr=" << addr;
            return eret;
        }
        exported = true;
    }
    struct DmabufCloser {
        RdmaContext::SharedDmabuf *exp;
        ~DmabufCloser() {
            if (exp) RdmaContext::closeSharedDmabuf(*exp);
        }
    } dmabuf_closer{exported ? &shared : nullptr};

    auto unregisterChunkMRs = [&](void *chunk_addr) {
        for (auto &context : context_list_) {
            int ret = context->unregisterMemoryRegion(chunk_addr);
            if (ret)
                LOG(WARNING) << "Rollback: failed to unregister chunk MR at "
                             << chunk_addr;
        }
    };
    auto rollbackChunks = [&](size_t committed) {
        size_t n = std::min(committed, chunks.size());
        for (size_t ri = 0; ri < n; ++ri) {
            metadata_->removeLocalMemoryBuffer(chunks[ri].first, false);
            unregisterChunkMRs(chunks[ri].first);
        }
        if (n > 0 && update_metadata) metadata_->updateLocalSegmentDesc();
    };

    for (size_t ci = 0; ci < chunks.size(); ++ci) {
        void *chunk_addr = chunks[ci].first;
        size_t chunk_len = chunks[ci].second;
        const uint64_t chunk_offset = reinterpret_cast<uintptr_t>(chunk_addr) -
                                      reinterpret_cast<uintptr_t>(addr);
        if (do_pre_touch) {
            int ret = preTouchMemory(chunk_addr, chunk_len);
            if (ret != 0) {
                rollbackChunks(ci);
                return ret;
            }
        }

        int use_parallel_reg = 0;
        if (!force_sequential) {
            use_parallel_reg = globalConfig().parallel_reg_mr;
            if (use_parallel_reg == -1) {
                use_parallel_reg = context_list_.size() > 1 && do_pre_touch;
            }
        }

        if (use_parallel_reg) {
            std::vector<std::thread> reg_threads;
            reg_threads.reserve(context_list_.size());
            std::vector<int> ret_codes(context_list_.size(), 0);
            const int ar = access_rights;
            for (size_t i = 0; i < context_list_.size(); ++i) {
                reg_threads.emplace_back([this, &ret_codes, &shared, i,
                                          chunk_addr, chunk_len, ar,
                                          chunk_offset]() {
                    ret_codes[i] = context_list_[i]->registerMemoryRegion(
                        chunk_addr, chunk_len, ar, shared, chunk_offset);
                });
            }
            for (auto &thread : reg_threads) thread.join();
            for (size_t i = 0; i < ret_codes.size(); ++i) {
                if (ret_codes[i] != 0) {
                    LOG(ERROR) << "Failed to register memory region (chunk "
                               << ci << ") with context " << i;
                    unregisterChunkMRs(chunk_addr);
                    rollbackChunks(ci);
                    return ret_codes[i];
                }
            }
        } else {
            for (size_t i = 0; i < context_list_.size(); ++i) {
                int ret = context_list_[i]->registerMemoryRegion(
                    chunk_addr, chunk_len, access_rights, shared, chunk_offset);
                if (ret) {
                    LOG(ERROR) << "Failed to register memory region (chunk "
                               << ci << ") with context " << i;
                    unregisterChunkMRs(chunk_addr);
                    rollbackChunks(ci);
                    return ret;
                }
            }
        }

        BufferDesc buffer_desc;
        for (auto &context : context_list_) {
            buffer_desc.lkey.push_back(context->lkey(chunk_addr));
            buffer_desc.rkey.push_back(context->rkey(chunk_addr));
        }
        buffer_desc.name = resolved_name;
        buffer_desc.addr = (uint64_t)chunk_addr;
        buffer_desc.length = chunk_len;
#ifdef ENABLE_MULTI_PROTOCOL
        buffer_desc.protocol = "rdma";
#endif
        int rc = metadata_->addLocalMemoryBuffer(buffer_desc, false);
        if (rc) {
            unregisterChunkMRs(chunk_addr);
            rollbackChunks(ci);
            return rc;
        }
    }

    if (update_metadata) {
        int rc = metadata_->updateLocalSegmentDesc();
        if (rc) {
            rollbackChunks(chunks.size());
            return rc;
        }
    }
    if (chunks.size() > 1) {
        std::lock_guard<std::mutex> lock(chunk_map_mutex_);
        std::vector<uint64_t> chunk_addrs;
        chunk_addrs.reserve(chunks.size());
        for (auto &c : chunks) chunk_addrs.push_back((uint64_t)c.first);
        chunk_map_[(uint64_t)addr] = std::move(chunk_addrs);
    }
    return 0;
}

'''
tt = tt[:start] + new_body + tt[end:]

unreg_anchor = "int RdmaTransport::unregisterLocalMemoryInternal(void *addr,\n                                                 bool update_metadata,\n                                                 bool force_sequential) {\n"
unreg_insert = unreg_anchor + r'''    std::vector<uint64_t> chunk_addrs;
    {
        std::lock_guard<std::mutex> lock(chunk_map_mutex_);
        auto it = chunk_map_.find((uint64_t)addr);
        if (it != chunk_map_.end()) {
            chunk_addrs = std::move(it->second);
            chunk_map_.erase(it);
        }
    }
    if (!chunk_addrs.empty()) {
        int first_err = 0;
        for (uint64_t ca : chunk_addrs) {
            int rc = metadata_->removeLocalMemoryBuffer(
                reinterpret_cast<void *>(ca), false);
            if (rc && !first_err) first_err = rc;
        }
        for (uint64_t ca : chunk_addrs) {
            for (auto &context : context_list_) {
                int ret = context->unregisterMemoryRegion(
                    reinterpret_cast<void *>(ca));
                if (ret && !first_err) first_err = ret;
            }
        }
        if (update_metadata) {
            int rc = metadata_->updateLocalSegmentDesc();
            if (rc && !first_err) first_err = rc;
        }
        return first_err;
    }

'''
if unreg_anchor not in tt:
    raise SystemExit("unregister anchor missing")
tt = tt.replace(unreg_anchor, unreg_insert, 1)

# HIP and RDMA share one buffer list. A HIP entry has no RDMA lkeys, so
# selecting it and indexing lkey[device_id] walks off the vector.
buffer_loop = "        const auto &buffer = buffers[buffer_id];\n"
protocol_skip = (
    buffer_loop
    + "#ifdef ENABLE_MULTI_PROTOCOL\n"
    + "        // Skip entries registered by another protocol.\n"
    + '        if (!buffer.protocol.empty() && buffer.protocol != "rdma") continue;\n'
    + "#endif\n"
)
if tt.count(buffer_loop) != 2:
    raise SystemExit("selectDevice buffer loops missing")
tt = tt.replace(buffer_loop, protocol_skip)
transport.write_text(tt)
print("patched")

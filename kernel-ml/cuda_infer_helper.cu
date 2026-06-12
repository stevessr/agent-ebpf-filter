// CUDA userspace inference helper for the kernel-ml DKMS module.
//
// Linux kernel modules cannot call libcuda/libcudart directly.  This helper
// keeps CUDA in userspace, mirrors the model that was loaded through
// /proc/ml_load, consumes binary requests from /proc/ml_cuda_request, and
// writes binary results to /proc/ml_cuda_result.

#include <cuda_runtime.h>

#include <cerrno>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <string>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <vector>

#include "ml_inference.h"

namespace {

constexpr const char* kDefaultRequestPath = "/proc/ml_cuda_request";
constexpr const char* kDefaultResultPath = "/proc/ml_cuda_result";
constexpr const char* kDefaultModelPath = "/proc/ml_cuda_model";
constexpr uint32_t kMaxTrees = NUM_TREES;
constexpr uint32_t kMaxTreeNodes = MAX_TREE_NODES;

volatile sig_atomic_t g_stop = 0;

struct HostModel {
    uint32_t version = 0;
    uint32_t numTrees = 0;
    uint32_t featureDim = 0;
    uint32_t numClasses = ML_DEFAULT_NUM_CLASSES;
    uint32_t maxDepth = ML_DEFAULT_MAX_DEPTH;
    uint64_t generation = UINT64_MAX;
    std::vector<tree_node> nodes;
    std::vector<int32_t> offsets;
    std::vector<int32_t> counts;
};

struct DeviceModel {
    tree_node* nodes = nullptr;
    int32_t* offsets = nullptr;
    int32_t* counts = nullptr;
    uint32_t numTrees = 0;
    uint32_t numClasses = ML_DEFAULT_NUM_CLASSES;
    uint32_t maxDepth = ML_DEFAULT_MAX_DEPTH;
};

void onSignal(int) { g_stop = 1; }

std::string errnoString(const char* what) {
    std::string out = what;
    out += ": ";
    out += std::strerror(errno);
    return out;
}

bool readExactFD(int fd, void* data, size_t size) {
    auto* p = static_cast<uint8_t*>(data);
    size_t off = 0;
    while (off < size) {
        ssize_t n = ::read(fd, p + off, size - off);
        if (n == 0) return false;
        if (n < 0) {
            if (errno == EINTR && !g_stop) continue;
            return false;
        }
        off += static_cast<size_t>(n);
    }
    return true;
}

bool writeExactFD(int fd, const void* data, size_t size) {
    const auto* p = static_cast<const uint8_t*>(data);
    size_t off = 0;
    while (off < size) {
        ssize_t n = ::write(fd, p + off, size - off);
        if (n < 0) {
            if (errno == EINTR && !g_stop) continue;
            return false;
        }
        off += static_cast<size_t>(n);
    }
    return true;
}

bool readFile(const std::string& path, std::vector<uint8_t>& out, bool quiet = false) {
    int fd = ::open(path.c_str(), O_RDONLY | O_CLOEXEC);
    if (fd < 0) {
        if (!quiet) std::fprintf(stderr, "kernel-ml-cuda: %s\n", errnoString(("open " + path).c_str()).c_str());
        return false;
    }

    out.clear();
    uint8_t buf[64 * 1024];
    while (true) {
        ssize_t n = ::read(fd, buf, sizeof(buf));
        if (n == 0) break;
        if (n < 0) {
            if (errno == EINTR && !g_stop) continue;
            if (!quiet) std::fprintf(stderr, "kernel-ml-cuda: %s\n", errnoString(("read " + path).c_str()).c_str());
            ::close(fd);
            return false;
        }
        out.insert(out.end(), buf, buf + n);
    }
    ::close(fd);
    return !out.empty();
}

template <typename T>
bool readScalar(const std::vector<uint8_t>& raw, size_t& off, T& value) {
    if (off + sizeof(T) > raw.size()) return false;
    std::memcpy(&value, raw.data() + off, sizeof(T));
    off += sizeof(T);
    return true;
}

bool parseModelBlob(const std::vector<uint8_t>& raw, uint64_t generation, HostModel& model, std::string& err) {
    if (raw.size() < 4 * sizeof(uint32_t)) {
        err = "model blob is too small";
        return false;
    }

    size_t off = 0;
    uint32_t totalNodes = 0;
    HostModel next;
    if (!readScalar(raw, off, next.version) || !readScalar(raw, off, next.numTrees) ||
        !readScalar(raw, off, next.featureDim) || !readScalar(raw, off, totalNodes)) {
        err = "failed to read model header";
        return false;
    }
    (void)totalNodes;

    if (next.numTrees == 0 || next.numTrees > kMaxTrees) {
        err = "model has invalid tree count";
        return false;
    }
    if (next.featureDim != FEATURE_DIM) {
        err = "model feature dimension mismatch";
        return false;
    }
    if (next.version >= 2) {
        if (!readScalar(raw, off, next.numClasses) || !readScalar(raw, off, next.maxDepth)) {
            err = "truncated v2 model header";
            return false;
        }
    }
    if (next.numClasses == 0 || next.numClasses > ML_MAX_CLASSES) {
        err = "model has invalid class count";
        return false;
    }
    if (next.maxDepth == 0 || next.maxDepth > ML_MAX_TREE_DEPTH) {
        err = "model has invalid max depth";
        return false;
    }

    next.offsets.reserve(next.numTrees);
    next.counts.reserve(next.numTrees);
    for (uint32_t t = 0; t < next.numTrees; ++t) {
        uint32_t count = 0;
        if (!readScalar(raw, off, count)) {
            err = "truncated tree node count";
            return false;
        }
        if (count == 0 || count > kMaxTreeNodes) {
            err = "tree node count is outside safety limits";
            return false;
        }
        if (off + static_cast<size_t>(count) * sizeof(tree_node) > raw.size()) {
            err = "truncated tree node array";
            return false;
        }
        next.offsets.push_back(static_cast<int32_t>(next.nodes.size()));
        next.counts.push_back(static_cast<int32_t>(count));
        size_t old = next.nodes.size();
        next.nodes.resize(old + count);
        std::memcpy(next.nodes.data() + old, raw.data() + off, static_cast<size_t>(count) * sizeof(tree_node));
        off += static_cast<size_t>(count) * sizeof(tree_node);
    }

    next.generation = generation;
    model = std::move(next);
    return true;
}

void freeDeviceModel(DeviceModel& dev) {
    if (dev.nodes) cudaFree(dev.nodes);
    if (dev.offsets) cudaFree(dev.offsets);
    if (dev.counts) cudaFree(dev.counts);
    dev = DeviceModel{};
}

bool uploadModel(const HostModel& host, DeviceModel& dev, std::string& err) {
    freeDeviceModel(dev);
    if (host.nodes.empty() || host.offsets.empty() || host.counts.empty()) {
        err = "host model is empty";
        return false;
    }

    cudaError_t ce;
    ce = cudaMalloc(reinterpret_cast<void**>(&dev.nodes), host.nodes.size() * sizeof(tree_node));
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); return false; }
    ce = cudaMalloc(reinterpret_cast<void**>(&dev.offsets), host.offsets.size() * sizeof(int32_t));
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); freeDeviceModel(dev); return false; }
    ce = cudaMalloc(reinterpret_cast<void**>(&dev.counts), host.counts.size() * sizeof(int32_t));
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); freeDeviceModel(dev); return false; }

    ce = cudaMemcpy(dev.nodes, host.nodes.data(), host.nodes.size() * sizeof(tree_node), cudaMemcpyHostToDevice);
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); freeDeviceModel(dev); return false; }
    ce = cudaMemcpy(dev.offsets, host.offsets.data(), host.offsets.size() * sizeof(int32_t), cudaMemcpyHostToDevice);
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); freeDeviceModel(dev); return false; }
    ce = cudaMemcpy(dev.counts, host.counts.data(), host.counts.size() * sizeof(int32_t), cudaMemcpyHostToDevice);
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); freeDeviceModel(dev); return false; }

    dev.numTrees = host.numTrees;
    dev.numClasses = host.numClasses;
    dev.maxDepth = host.maxDepth;
    return true;
}

bool loadAndUploadModel(const std::string& path, uint64_t generation, HostModel& host, DeviceModel& dev, bool quiet = false) {
    std::vector<uint8_t> raw;
    std::string err;
    if (!readFile(path, raw, quiet)) return false;
    if (!parseModelBlob(raw, generation, host, err)) {
        if (!quiet) std::fprintf(stderr, "kernel-ml-cuda: parse model %s failed: %s\n", path.c_str(), err.c_str());
        return false;
    }
    if (!uploadModel(host, dev, err)) {
        if (!quiet) std::fprintf(stderr, "kernel-ml-cuda: upload model failed: %s\n", err.c_str());
        return false;
    }
    std::fprintf(stderr,
                 "kernel-ml-cuda: loaded model gen=%llu version=%u trees=%u nodes=%zu classes=%u max_depth=%u from %s\n",
                 static_cast<unsigned long long>(host.generation), host.version,
                 host.numTrees, host.nodes.size(), host.numClasses, host.maxDepth, path.c_str());
    return true;
}

__device__ int traverseDeviceTree(const tree_node* nodes, int32_t count, const feature_vector* fv, uint32_t maxDepth) {
    int32_t idx = 0;
    uint32_t depth = 0;
    if (maxDepth == 0 || maxDepth > ML_MAX_TREE_DEPTH) maxDepth = ML_DEFAULT_MAX_DEPTH;
    while (idx >= 0 && idx < count && depth++ < maxDepth) {
        const tree_node& node = nodes[idx];
        if (node.is_leaf) {
            if (node.leaf_value >= 0 && node.leaf_value < ML_MAX_CLASSES)
                return node.leaf_value;
            return ML_ACTION_ALLOW;
        }
        if (node.feature_idx >= FEATURE_DIM) return ML_ACTION_ALLOW;
        int64_t feature = fv->features[node.feature_idx];
        idx = (feature < node.threshold) ? node.left_child : node.right_child;
    }
    return ML_ACTION_ALLOW;
}

__global__ void rfPredictKernel(const tree_node* nodes, const int32_t* offsets, const int32_t* counts,
                                uint32_t numTrees, uint32_t numClasses, uint32_t maxDepth,
                                const feature_vector* fv, int* outAction) {
    __shared__ int votes[ML_MAX_CLASSES];
    if (threadIdx.x < ML_MAX_CLASSES) votes[threadIdx.x] = 0;
    __syncthreads();
    if (numClasses == 0 || numClasses > ML_MAX_CLASSES) numClasses = ML_DEFAULT_NUM_CLASSES;

    int t = threadIdx.x;
    if (t < static_cast<int>(numTrees)) {
        int32_t base = offsets[t];
        int32_t count = counts[t];
        int action = traverseDeviceTree(nodes + base, count, fv, maxDepth);
        if (action >= 0 && action < static_cast<int>(numClasses))
            atomicAdd(&votes[action], 1);
    }
    __syncthreads();

    if (threadIdx.x == 0) {
        int best = ML_ACTION_ALLOW;
        for (uint32_t c = 1; c < numClasses; ++c) {
            if (votes[c] > votes[best]) best = static_cast<int>(c);
        }
        *outAction = best;
    }
}

bool cudaPredict(const DeviceModel& dev, const feature_vector& fv, uint32_t& action, std::string& err) {
    if (!dev.nodes || !dev.offsets || !dev.counts || dev.numTrees == 0) {
        err = "no model loaded";
        return false;
    }

    feature_vector* dFv = nullptr;
    int* dAction = nullptr;
    cudaError_t ce = cudaMalloc(reinterpret_cast<void**>(&dFv), sizeof(feature_vector));
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); return false; }
    ce = cudaMalloc(reinterpret_cast<void**>(&dAction), sizeof(int));
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); cudaFree(dFv); return false; }
    ce = cudaMemcpy(dFv, &fv, sizeof(feature_vector), cudaMemcpyHostToDevice);
    if (ce != cudaSuccess) { err = cudaGetErrorString(ce); cudaFree(dFv); cudaFree(dAction); return false; }

    int threads = 32;
    rfPredictKernel<<<1, threads>>>(dev.nodes, dev.offsets, dev.counts,
                                    dev.numTrees, dev.numClasses, dev.maxDepth,
                                    dFv, dAction);
    ce = cudaGetLastError();
    if (ce == cudaSuccess) ce = cudaDeviceSynchronize();
    int hostAction = ML_ACTION_ALLOW;
    if (ce == cudaSuccess) ce = cudaMemcpy(&hostAction, dAction, sizeof(int), cudaMemcpyDeviceToHost);

    cudaFree(dFv);
    cudaFree(dAction);

    if (ce != cudaSuccess) {
        err = cudaGetErrorString(ce);
        return false;
    }
    if (hostAction < 0 || hostAction >= static_cast<int>(dev.numClasses)) hostAction = ML_ACTION_ALLOW;
    action = static_cast<uint32_t>(hostAction);
    return true;
}

void usage(const char* argv0) {
    std::fprintf(stderr,
        "Usage: %s [--model model.bin] [--request /proc/ml_cuda_request] "
        "[--result /proc/ml_cuda_result] [--proc-model /proc/ml_cuda_model] "
        "[--self-test] [--oneshot]\n\n"
        "If --model is omitted, the helper mirrors the latest /proc/ml_load blob "
        "from --proc-model whenever request.model_generation changes.\n",
        argv0);
}

bool runSelfTest() {
    HostModel host;
    host.version = 1;
    host.numTrees = 1;
    host.featureDim = FEATURE_DIM;
    host.generation = 0;
    host.offsets.push_back(0);
    host.counts.push_back(3);
    host.nodes.resize(3);
    host.nodes[0].feature_idx = 0;
    host.nodes[0].threshold = FLOAT_SCALE;
    host.nodes[0].left_child = 1;
    host.nodes[0].right_child = 2;
    host.nodes[1].is_leaf = 1;
    host.nodes[1].leaf_value = ML_ACTION_BLOCK;
    host.nodes[2].is_leaf = 1;
    host.nodes[2].leaf_value = ML_ACTION_ALLOW;

    DeviceModel dev;
    std::string err;
    if (!uploadModel(host, dev, err)) {
        std::fprintf(stderr, "kernel-ml-cuda: self-test upload failed: %s\n", err.c_str());
        return false;
    }

    feature_vector fv{};
    uint32_t action = ML_ACTION_ALERT;
    fv.features[0] = 0;
    bool ok = cudaPredict(dev, fv, action, err) && action == ML_ACTION_BLOCK;
    fv.features[0] = 2 * FLOAT_SCALE;
    ok = ok && cudaPredict(dev, fv, action, err) && action == ML_ACTION_ALLOW;
    freeDeviceModel(dev);

    if (!ok) {
        std::fprintf(stderr, "kernel-ml-cuda: self-test failed: action=%u err=%s\n",
                     action, err.c_str());
        return false;
    }
    std::printf("kernel-ml-cuda: self-test passed\n");
    return true;
}

}  // namespace

int main(int argc, char** argv) {
    std::string modelPath;
    std::string requestPath = kDefaultRequestPath;
    std::string resultPath = kDefaultResultPath;
    std::string procModelPath = kDefaultModelPath;
    bool selfTest = false;
    bool oneshot = false;

    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        auto needValue = [&](const char* name) -> const char* {
            if (i + 1 >= argc) {
                usage(argv[0]);
                std::exit(2);
            }
            return argv[++i];
        };
        if (arg == "--model") modelPath = needValue("--model");
        else if (arg == "--request") requestPath = needValue("--request");
        else if (arg == "--result") resultPath = needValue("--result");
        else if (arg == "--proc-model") procModelPath = needValue("--proc-model");
        else if (arg == "--self-test") selfTest = true;
        else if (arg == "--oneshot") oneshot = true;
        else if (arg == "-h" || arg == "--help") { usage(argv[0]); return 0; }
        else { usage(argv[0]); return 2; }
    }

    std::signal(SIGINT, onSignal);
    std::signal(SIGTERM, onSignal);

    static_assert(sizeof(tree_node) == 32, "tree_node UAPI must stay 32 bytes");
    static_assert(sizeof(feature_vector) == 1048, "feature_vector UAPI changed");
    static_assert(sizeof(ml_cuda_request) == 1072, "ml_cuda_request UAPI changed");
    static_assert(sizeof(ml_cuda_result) == 24, "ml_cuda_result UAPI changed");

    int devices = 0;
    cudaError_t ce = cudaGetDeviceCount(&devices);
    if (ce != cudaSuccess || devices <= 0) {
        std::fprintf(stderr, "kernel-ml-cuda: no CUDA device available: %s\n", cudaGetErrorString(ce));
        return 1;
    }
    cudaDeviceProp prop{};
    cudaGetDeviceProperties(&prop, 0);
    std::fprintf(stderr, "kernel-ml-cuda: using CUDA device 0: %s\n", prop.name);

    if (selfTest) {
        return runSelfTest() ? 0 : 1;
    }

    HostModel host;
    DeviceModel dev;
    if (!modelPath.empty()) {
        if (!loadAndUploadModel(modelPath, 0, host, dev)) return 1;
    } else {
        // It is OK if no model has been loaded yet; the first request with a
        // non-zero generation will trigger a reload from /proc/ml_cuda_model.
        (void)loadAndUploadModel(procModelPath, 0, host, dev, true);
    }

    int reqFD = ::open(requestPath.c_str(), O_RDONLY | O_CLOEXEC);
    if (reqFD < 0) {
        std::fprintf(stderr, "kernel-ml-cuda: %s\n", errnoString(("open " + requestPath).c_str()).c_str());
        freeDeviceModel(dev);
        return 1;
    }
    int resFD = ::open(resultPath.c_str(), O_WRONLY | O_CLOEXEC);
    if (resFD < 0) {
        std::fprintf(stderr, "kernel-ml-cuda: %s\n", errnoString(("open " + resultPath).c_str()).c_str());
        ::close(reqFD);
        freeDeviceModel(dev);
        return 1;
    }

    bool processed = false;
    while (!g_stop) {
        ml_cuda_request req{};
        if (!readExactFD(reqFD, &req, sizeof(req))) {
            if (!g_stop) std::fprintf(stderr, "kernel-ml-cuda: failed to read CUDA request\n");
            break;
        }
        ml_cuda_result result{};
        result.version = ML_CUDA_REQUEST_VERSION;
        result.request_id = req.request_id;
        result.action = ML_ACTION_ALLOW;

        if (req.version != ML_CUDA_REQUEST_VERSION) {
            result.status = EPROTO;
        } else {
            bool haveModel = dev.nodes != nullptr;
            if (modelPath.empty() && req.model_generation != host.generation) {
                haveModel = loadAndUploadModel(procModelPath, req.model_generation, host, dev, false);
            }
            if (!haveModel) {
                result.status = ENODATA;
            } else {
                std::string err;
                uint32_t action = ML_ACTION_ALLOW;
                if (cudaPredict(dev, req.features, action, err)) {
                    result.action = action;
                    result.status = 0;
                } else {
                    std::fprintf(stderr, "kernel-ml-cuda: inference failed for request %llu: %s\n",
                                 static_cast<unsigned long long>(req.request_id), err.c_str());
                    result.status = EIO;
                }
            }
        }

        if (!writeExactFD(resFD, &result, sizeof(result))) {
            if (!g_stop) std::fprintf(stderr, "kernel-ml-cuda: failed to write CUDA result\n");
            break;
        }
        processed = true;
        if (oneshot)
            break;
    }

    ::close(resFD);
    ::close(reqFD);
    freeDeviceModel(dev);
    return (g_stop || (oneshot && processed)) ? 0 : 1;
}

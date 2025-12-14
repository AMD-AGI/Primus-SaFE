#!/bin/bash 
## Script version 1.0 
## RUN THIS FROM THE FOLDER YOU WANT ALL THE TOOLS INSTALLED 
## Script takes current working directory, set the install/target directory to what you want 
WORKDIR=/opt 

TIMESTAMP=$(date +"%Y%m%d_%H%M%S") 
LOG_FILE="install_log_${TIMESTAMP}.log" 

# --- Configuration Variables --- 
## change this if ROCm is installed in a non-standard path 
## Setting this is required for correct URL parsing 
ROCM_PATH=/opt/rocm 
UCX_VERSION="1.15.0" 
MPI_VERSION="4.1.6" 
RCCL_VERSION="rocm-7.1.1"
ANP_VERSION="tags/v1.3.0"
AINIC_BUNDLE_VERSION="1.117.5-a-38"

AINIC_BUNDLE_FILE="./ainic_bundle_${AINIC_BUNDLE_VERSION}.tar.gz"
## install ainic driver
echo "AINIC_BUNDLE_FILE: ${AINIC_BUNDLE_FILE}"
sleep 10

cp $AINIC_BUNDLE_FILE $WORKDIR
cd $WORKDIR
apt install jq dpkg-dev kmod xz-utils  -y 
sudo apt install -y libibverbs-dev ibverbs-utils infiniband-diags -y 
apt install initramfs-tools -y
tar zxf $AINIC_BUNDLE_VERSION.tar.gz
tar zxf host_sw_pkg.tar.gz
cd host_sw_pkg && ./install.sh --domain=user -y
cd /opt
#ibv_devices


## Auto-configured based on previous step 
UCX_TARBALL="ucx-${UCX_VERSION}.tar.gz" 
UCX_DOWNLOAD_URL="https://github.com/openucx/ucx/releases/download/v${UCX_VERSION}/${UCX_TARBALL}" 
UCX_DIR="ucx-${UCX_VERSION}" 
MPI_TARBALL="openmpi-${MPI_VERSION}.tar.gz" 
MPI_DOWNLOAD_URL="https://download.open-mpi.org/release/open-mpi/v$(echo "${MPI_VERSION}" | cut -d. -f1-2)/openmpi-${MPI_VERSION}.tar.gz" 
MPI_DIR="ompi-${MPI_VERSION}" 

# Install UCX
cd ${WORKDIR}
echo "Downloading and building UCX ${UCX_VERSION}..." 
wget "${UCX_DOWNLOAD_URL}"
mkdir -p "${UCX_DIR}"
tar -zxf "${UCX_TARBALL}" -C "${UCX_DIR}" --strip-components=1
cd "${UCX_DIR}"
mkdir -p build
cd build
../configure --prefix="${WORKDIR}/${UCX_DIR}/install" --with-rocm="${ROCM_PATH}"
make -j16
make install
echo "UCX ${UCX_VERSION} built and installed successfully." 
export UCX_INSTALL_DIR="${WORKDIR}/${UCX_DIR}/install" 
echo "UCX_INSTALL_DIR set to: ${UCX_INSTALL_DIR}"

# Install OpenMPI
cd ${WORKDIR}
wget "${MPI_DOWNLOAD_URL}"
mkdir -p "${MPI_DIR}"
tar -zxf "${MPI_TARBALL}" -C "${MPI_DIR}" --strip-components=1 
cd "${MPI_DIR}" 
mkdir build
cd build
../configure --prefix="${WORKDIR}/${MPI_DIR}/install" --with-ucx="${UCX_INSTALL_DIR}" --disable-oshmem --disable-mpi-fortran
make -j16
make install
echo "OpenMPI ${MPI_VERSION} built and installed successfully." 
export MPI_INSTALL_DIR="${WORKDIR}/${MPI_DIR}/install" 
echo "MPI_INSTALL_DIR set to: ${MPI_INSTALL_DIR}"


# install RCCL
cd ${WORKDIR}
# Use HTTPS for cloning (no SSH key required)
git clone https://github.com/ROCm/rccl.git
cd rccl
git checkout ${RCCL_VERSION}
./install.sh -l --prefix build/ --disable-msccl-kernel 


# build amd-anp
#Set path to RCCL source
export RCCL_HOME=${WORKDIR}/rccl
#Set OMPI lib path
export MPI_LIB_PATH=${WORKDIR}/${MPI_DIR}/build/ompi/.libs/
#Set OMPI include path
export MPI_INCLUDE=${WORKDIR}/${MPI_DIR}/install/include


# build amd-anp
cd ${WORKDIR}
apt update && apt install -y libfmt-dev
# Use HTTPS for cloning (no SSH key required)
git clone https://github.com/rocm/amd-anp.git
cd amd-anp
git checkout ${ANP_VERSION}
make -j 16 RCCL_HOME=$RCCL_HOME MPI_INCLUDE=$MPI_INCLUDE MPI_LIB_PATH=$MPI_LIB_PATH  ROCM_PATH=${ROCM_PATH}
# install the plugin
make RCCL_HOME=$RCCL_HOME ROCM_PATH=${ROCM_PATH} install
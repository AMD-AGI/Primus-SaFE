"""
TraceLens Trace Analyzer - Streamlit UI Entry Point

This script provides the main entry point for the TraceLens Streamlit application.
It loads trace files and displays analysis using the TraceLens library.

Environment Variables:
    SESSION_ID: Unique session identifier
    TRACE_FILE_PATH: Path to the trace file to analyze
    PROFILER_FILE_ID: ID of the profiler file (for API integration)
    API_BASE_URL: Base URL for the Lens API (default: http://lens-api:8080)
    BASE_URL_PATH: Base URL path for reverse proxy (e.g., /api/v1/tracelens/sessions/xxx/ui)
"""

import os
import sys
import gzip
import json
from io import BytesIO
from dataclasses import dataclass
from typing import Optional

import streamlit as st
import pandas as pd

# Get configuration from environment variables
SESSION_ID = os.getenv("SESSION_ID", "unknown")
TRACE_FILE_PATH = os.getenv("TRACE_FILE_PATH", "")
PROFILER_FILE_ID = os.getenv("PROFILER_FILE_ID", "")
API_BASE_URL = os.getenv("API_BASE_URL", "http://lens-api:8080")
BASE_URL_PATH = os.getenv("BASE_URL_PATH", "")

# Configure Streamlit page
st.set_page_config(
    page_title=f"TraceLens - {SESSION_ID}",
    page_icon="🔍",
    layout="wide",
    initial_sidebar_state="expanded"
)


def load_trace_from_file(file_path: str):
    """Load trace data from a local file path."""
    if not os.path.exists(file_path):
        raise FileNotFoundError(f"Trace file not found: {file_path}")
    
    # Return the file path for TraceLens to load
    return file_path


def load_trace_from_api(file_id: str):
    """Load trace data from the Lens API using streaming."""
    import requests
    
    url = f"{API_BASE_URL}/v1/profiler/files/{file_id}/content"
    # Use streaming to avoid loading entire response into memory at once
    response = requests.get(url, timeout=300, stream=True)
    response.raise_for_status()
    return response


@dataclass
class ExperimentNames:
    BASELINE = "Baseline"
    EXPERIMENT = "Experiment"


def is_gzip_content(data: bytes) -> bool:
    """Check if data is gzip compressed by checking magic bytes."""
    return len(data) >= 2 and data[0:2] == b'\x1f\x8b'


def parse_trace_events_streaming(file_obj):
    """
    Parse only traceEvents from JSON using ijson for memory efficiency.
    Falls back to standard json if ijson is not available.
    """
    try:
        import ijson
        # Use ijson to stream parse only traceEvents array
        trace_events = list(ijson.items(file_obj, 'traceEvents.item'))
        return trace_events
    except ImportError:
        # Fallback to standard json
        file_obj.seek(0)
        data = json.load(file_obj)
        return data.get("traceEvents", [])


def analyze_single_trace(trace_data, file_name: str):
    """Analyze a single trace file and display results."""
    try:
        from TraceLens.Trace2Tree.trace_to_tree import TraceToTree
        from TraceLens.TreePerf.tree_perf import TreePerfAnalyzer
        
        # Handle both BytesIO and requests.Response objects
        if hasattr(trace_data, 'content'):
            # It's a requests.Response - get raw content
            raw_content = trace_data.content
            trace_data = BytesIO(raw_content)
        elif hasattr(trace_data, 'read'):
            trace_data.seek(0)
            raw_content = trace_data.read()
            trace_data = BytesIO(raw_content)
        else:
            raw_content = trace_data
            trace_data = BytesIO(raw_content)
        
        # Check if gzip and decompress using streaming
        if is_gzip_content(raw_content):
            # Use streaming gzip decompression
            file_obj = gzip.GzipFile(fileobj=BytesIO(raw_content))
        else:
            file_obj = BytesIO(raw_content)
        
        # Parse trace events - try streaming first for memory efficiency
        st.info("Parsing trace file (this may take a moment for large files)...")
        trace_events = parse_trace_events_streaming(file_obj)
        
        if not trace_events:
            st.error("No trace events found in file")
            return False
        
        st.success(f"Loaded {len(trace_events):,} trace events")
        
        # Create analyzer
        tree = TraceToTree(trace_events)
        analyzer = TreePerfAnalyzer(tree)
        
        # Clear trace_events to free memory
        del trace_events
        
        # Get kernel data
        kernels = analyzer.get_df_kernel_launchers()
        kernels_summary = analyzer.get_df_kernel_launchers_summary(kernels)
        gemms_summary = analyzer.get_df_kernel_launchers_summary_by_shape(kernels, "aten::mm")
        
        # Display tabs
        summary_tab, gemms_tab, fa_tab, compare_tab = st.tabs([
            "Kernel Summary",
            "GEMMs Analysis", 
            "Flash Attention",
            "Compare with Another Trace"
        ])
        
        with summary_tab:
            st.subheader("Kernel Performance Summary")
            if not kernels_summary.empty:
                st.dataframe(
                    kernels_summary.round().astype(int, errors="ignore"),
                    use_container_width=True
                )
            else:
                st.warning("No kernel data found in trace.")
        
        with gemms_tab:
            st.subheader("GEMM Operations (aten::mm)")
            if not gemms_summary.empty:
                st.dataframe(
                    gemms_summary.round().astype(int, errors="ignore"),
                    use_container_width=True
                )
            else:
                st.info("No GEMM operations found in trace.")
        
        with fa_tab:
            st.subheader("Flash Attention Analysis")
            # Find flash attention kernels
            fa_kernels = kernels_summary[
                kernels_summary.index.str.lower().str.contains("flash", na=False)
            ]
            if not fa_kernels.empty:
                st.dataframe(
                    fa_kernels.round().astype(int, errors="ignore"),
                    use_container_width=True
                )
            else:
                st.info("No Flash Attention kernels found in trace.")
        
        with compare_tab:
            st.subheader("Compare with Another Trace")
            st.info("Upload a second trace file to compare performance.")
            
            experiment_trace = st.file_uploader(
                "Experiment Trace",
                accept_multiple_files=False,
                type=["json", "gz"],
                key="experiment_upload"
            )
            
            if experiment_trace:
                try:
                    from TraceLens.UI.trace_analyser import analyse_trace
                    
                    # Use uploaded file for comparison
                    st.toast(f"Analyzing {experiment_trace.name}...")
                    exp_analyzer, exp_kernels, exp_gemms = analyse_trace(experiment_trace)
                    exp_summary = exp_analyzer.get_df_kernel_launchers_summary(exp_kernels)
                    
                    st.subheader("Comparison Results")
                    
                    col1, col2 = st.columns(2)
                    with col1:
                        st.markdown("**Baseline (from API)**")
                        st.dataframe(kernels_summary.round().astype(int, errors="ignore"))
                    
                    with col2:
                        st.markdown(f"**Experiment ({experiment_trace.name})**")
                        st.dataframe(exp_summary.round().astype(int, errors="ignore"))
                        
                except Exception as e:
                    st.error(f"Failed to analyze experiment trace: {e}")
        
        return True
        
    except ImportError as e:
        st.error(f"Failed to import TraceLens components: {e}")
        return False
    except Exception as e:
        st.error(f"Error analyzing trace: {e}")
        st.exception(e)
        return False


def display_session_info():
    """Display session information in the sidebar."""
    with st.sidebar:
        st.markdown("### Session Info")
        st.text(f"Session ID: {SESSION_ID}")
        
        if TRACE_FILE_PATH:
            st.text(f"Trace File: {os.path.basename(TRACE_FILE_PATH)}")
        elif PROFILER_FILE_ID:
            st.text(f"Profiler File ID: {PROFILER_FILE_ID}")
        
        st.markdown("---")


def run_tracelens_analyzer():
    """Run the TraceLens analyzer UI."""
    try:
        # Import TraceLens
        from TraceLens.UI.trace_analyser import main as tracelens_main
        
        # TraceLens expects to be run as the main entry point
        # We set the trace file path in environment and call the main function
        tracelens_main()
        
    except ImportError as e:
        st.error(f"Failed to import TraceLens: {e}")
        st.info("Please ensure TraceLens is properly installed.")
        
        # Show manual file upload as fallback
        show_file_upload_fallback()
        
    except Exception as e:
        st.error(f"Error running TraceLens analyzer: {e}")
        st.exception(e)


def show_file_upload_fallback():
    """Show a file upload interface as fallback."""
    st.markdown("### Manual Trace Upload")
    st.markdown("Upload a trace file to analyze:")
    
    uploaded_file = st.file_uploader(
        "Choose a trace file",
        type=["json", "gz", "trace"],
        help="Upload a PyTorch/GPU trace file for analysis"
    )
    
    if uploaded_file is not None:
        st.success(f"Uploaded: {uploaded_file.name}")
        st.info("TraceLens analyzer not available. Please check installation.")


def main():
    """Main entry point for the TraceLens Streamlit application."""
    
    # Display header
    st.title("🔍 TraceLens Trace Analyzer")
    
    # Display session info in sidebar
    display_session_info()
    
    # Check if we have a trace file to analyze
    if TRACE_FILE_PATH:
        # Load from local file path
        try:
            trace_path = load_trace_from_file(TRACE_FILE_PATH)
            st.success(f"Loaded trace file: {os.path.basename(trace_path)}")
            
            # Read file and analyze
            with open(trace_path, "rb") as f:
                trace_data = BytesIO(f.read())
            
            if not analyze_single_trace(trace_data, os.path.basename(trace_path)):
                # Fallback to original TraceLens if single trace analysis fails
                os.environ["TRACE_FILE_PATH"] = trace_path
                run_tracelens_analyzer()
            
        except FileNotFoundError as e:
            st.error(str(e))
            show_file_upload_fallback()
            
    elif PROFILER_FILE_ID:
        # Load from API
        try:
            with st.spinner("Loading trace file from API..."):
                trace_data = load_trace_from_api(PROFILER_FILE_ID)
            st.success(f"Loaded trace file from API (ID: {PROFILER_FILE_ID})")
            
            # Get file info for display
            file_name = f"profiler_file_{PROFILER_FILE_ID}.json.gz"
            
            # Analyze the trace directly
            if not analyze_single_trace(trace_data, file_name):
                # Fallback: save to temp file and try original TraceLens
                import tempfile
                trace_data.seek(0)
                with tempfile.NamedTemporaryFile(delete=False, suffix=".json.gz") as f:
                    f.write(trace_data.read())
                    temp_path = f.name
                
                os.environ["TRACE_FILE_PATH"] = temp_path
                run_tracelens_analyzer()
            
        except Exception as e:
            st.error(f"Failed to load trace from API: {e}")
            st.exception(e)
            show_file_upload_fallback()
            
    else:
        # No trace file specified - show upload interface
        st.info("No trace file specified. Please upload a trace file or configure the session.")
        show_file_upload_fallback()


if __name__ == "__main__":
    main()


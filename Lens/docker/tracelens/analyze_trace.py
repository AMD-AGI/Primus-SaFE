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
import streamlit as st

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
    """Load trace data from the Lens API."""
    import requests
    from io import BytesIO
    
    url = f"{API_BASE_URL}/v1/profiler/files/{file_id}/content"
    response = requests.get(url, timeout=60)
    response.raise_for_status()
    return BytesIO(response.content)


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
            
            # Set environment for TraceLens
            os.environ["TRACE_FILE_PATH"] = trace_path
            
            # Run TraceLens analyzer
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
            
            # Save to temp file for TraceLens
            import tempfile
            with tempfile.NamedTemporaryFile(delete=False, suffix=".json") as f:
                f.write(trace_data.read())
                temp_path = f.name
            
            os.environ["TRACE_FILE_PATH"] = temp_path
            
            # Run TraceLens analyzer
            run_tracelens_analyzer()
            
        except Exception as e:
            st.error(f"Failed to load trace from API: {e}")
            show_file_upload_fallback()
            
    else:
        # No trace file specified - show upload interface
        st.info("No trace file specified. Please upload a trace file or configure the session.")
        show_file_upload_fallback()


if __name__ == "__main__":
    main()


#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.
import getpass
import json
import requests
import argparse
import os

class WorkloadSDK:
    def __init__(self, base_url):
        self.base_url = base_url.rstrip('/')
        self.session = requests.Session()
        self.session.headers.update({
            'Content-Type': 'application/json'
        })
        self.user_name_key = 'userName'

    def _handle_response(self, response):
        try:
            response.raise_for_status()
            return response.json()
        except requests.exceptions.HTTPError as e:
            raise Exception(f"API Error: {e}, Response: {response.text}")

    def create_workload_from_file(self, file_path):
        """
        Creates a workload from a JSON file.

        :param file_path: Path to the JSON file
        :return: Response content
        """
        if not os.path.isfile(file_path):
            raise FileNotFoundError(f"JSON file not found: {file_path}")

        try:
            with open(file_path, "r", encoding="utf-8") as f:
                payload: Dict[str, Any] = json.load(f)  # type: ignore[arg-type]
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON content in {file_path}: {e}") from e
        if not isinstance(payload, dict):
            raise ValueError("JSON root must be an object (dictionary)")

        user_name = payload.get(self.user_name_key)
        if not isinstance(user_name, str) or not user_name.strip():
            try:
                user_name = getpass.getuser()
            except Exception as e:  # pragma: no cover — system‑specific edge
                raise ValueError(f"Could not retrieve system username: {e}") from e

        if not user_name:
            raise ValueError("userName cannot be determined or is empty")

        payload[self.user_name_key] = user_name  # ensure key is present and valid

        url = f"{self.base_url}/api/v1/workloads"
        response = self.session.post(url, json=payload)
        return self._handle_response(response)

    def get_workload(self, workload_id):
        """
        Get workload details
        :param workload_id: id of the workload
        :return: Response content
        """
        url = f"{self.base_url}/api/v1/workloads/{workload_id}"
        response = self.session.get(url)
        return self._handle_response(response)

    def delete_workload(self, workload_id):
        """
        Delete workload
        :param workload_id: id of the workload
        :return
        """
        url = f"{self.base_url}/api/v1/workloads/{workload_id}"
        response = self.session.delete(url)
        return self._handle_response(response)



def main():
    parser = argparse.ArgumentParser(description="CLI tool for managing workloads via API")
    parser.add_argument("--url", required=True, help="Base URL of the API server (e.g., http://apiserver.safe.primus.ai)")

    subparsers = parser.add_subparsers(dest="command", required=True)

    # Create command
    create_parser = subparsers.add_parser("create", help="Create a new workload from JSON file")
    create_parser.add_argument("--json-file", required=True, help="Path to the JSON file containing the workload payload")

    # Get command
    get_parser = subparsers.add_parser("get", help="Get workload details")
    get_parser.add_argument("--workload-id", required=True, help="ID of the workload to retrieve")

    # Delete command
    delete_parser = subparsers.add_parser("delete", help="Delete a workload")
    delete_parser.add_argument("--workload-id", required=True, help="ID of the workload to delete")

    args = parser.parse_args()
    sdk = WorkloadSDK(base_url=args.url)

    try:
        if args.command == "create":
            result = sdk.create_workload_from_file(args.json_file)
            print("✅ workload created successfully:")
            print(json.dumps(result, indent=2))

        elif args.command == "get":
            result = sdk.get_workload(args.workload_id)
            print("🔍 workload details:")
            print(json.dumps(result, indent=2))

        elif args.command == "delete":
            result = sdk.delete_workload(args.workload_id)
            print("🗑️ workload deleted successfully:")
            print(json.dumps(result, indent=2))

    except Exception as e:
        print("❌ Error:", str(e))


if __name__ == "__main__":
    main()
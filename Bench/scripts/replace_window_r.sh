#!/bin/bash

find . -type f -name "*.sh" | while read file; do
    echo "$file"
    sed -i 's/\r$//' "$file"
done

echo "Completed"
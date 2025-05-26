#!/bin/bash

script_path="/shared-data/scripts"

while true; do
  if [ -d "$script_path" ] && [ "$(ls -A "$script_path")" ] ; then
      break
  else
      sleep 60
  fi
done


pids=()
for script_file in `ls $script_path`;
do
  /bin/bash $script_path/$script_file &
  pids+=($!)
done
wait -n "${pids[@]}"
exit $?
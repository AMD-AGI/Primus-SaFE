
IS_WSL := $(shell grep -q microsoft /proc/version && echo "yes" || echo "no")
shell_sed:
    ifeq ($(IS_WSL),yes)
		sed -i 's/\r//' bootstrap.sh
    endif

bootstrap: shell_sed


import wandb

_original_log = wandb.sdk.wandb_run.Run.log

def my_log(self, *args, **kwargs):
    # Your logic
    print("[MyWrapper] wandb.log called with:", args, kwargs)

    # Call original log
    return _original_log(self, *args, **kwargs)

# Replace
wandb.sdk.wandb_run.Run.log = my_log
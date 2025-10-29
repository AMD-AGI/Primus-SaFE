import wandb

_original_log = wandb.sdk.wandb_run.Run.log

def my_log(self, *args, **kwargs):
    # 你的逻辑
    print("[MyWrapper] wandb.log called with:", args, kwargs)

    # 调用原始的 log
    return _original_log(self, *args, **kwargs)

# 替换掉
wandb.sdk.wandb_run.Run.log = my_log
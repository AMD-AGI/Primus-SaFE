import torch
import torch.distributed as dist
import torch.nn as nn
import torch.optim as optim
import os

# -----------------------------
# Simulate DeepSeek + LLaMA layers
# -----------------------------
class DummyDeepSeekLayer(nn.Module):
    def __init__(self, hidden_dim, seq_len):
        super().__init__()
        self.seq_len = seq_len
        self.hidden_dim = hidden_dim
        self.linear = nn.Linear(hidden_dim, hidden_dim)
    
    def forward(self, x):
        return self.linear(x)

class DummyLLaMA405B(nn.Module):
    def __init__(self, hidden_dim=8192, seq_len=2048, num_layers=2):
        super().__init__()
        self.layers = nn.ModuleList([DummyDeepSeekLayer(hidden_dim, seq_len) for _ in range(num_layers)])
    
    def forward(self, x):
        for layer in self.layers:
            x = layer(x)
        return x

def run_dummy_training(rank, world_size, num_steps=3, hidden_dim=8192, seq_len=2048, num_layers=2):
    """
    Simulate DeepSeek v3 + LLaMA 3.1 405B training communication
    rank, world_size: initialized distributed information
    num_steps: simulated training steps
    """
    # Model and optimizer
    model = DummyLLaMA405B(hidden_dim=hidden_dim, seq_len=seq_len, num_layers=num_layers).cuda()
    optimizer = optim.Adam(model.parameters(), lr=1e-4)
    
    # Multi-step training simulation
    for step in range(num_steps):
        batch_size = 4
        # Random input to simulate token embedding
        x = torch.randn(batch_size, seq_len, hidden_dim).cuda()
        
        # forward
        y = model(x)
        loss = y.mean()
        
        # backward
        optimizer.zero_grad()
        loss.backward()
        
        # Simulate gradient All-Reduce (distributed gradient synchronization)
        for param in model.parameters():
            if param.grad is not None:
                dist.all_reduce(param.grad.data, op=dist.ReduceOp.SUM)
                param.grad.data /= world_size
        
        # Update parameters
        optimizer.step()
        
        if rank == 0:
            print(f"[Step {step+1}/{num_steps}] Loss={loss.item():.4f}")

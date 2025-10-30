from typing import Dict, Any, AsyncGenerator, List
from pulsekit.core.node import NodeInfo

class Executor:
    async def run(self, params: Dict[str, Any]) -> AsyncGenerator[str, None]:
        raise NotImplementedError
    
    def schedule(self, nodes: List[NodeInfo]) -> Dict:
        raise NotImplementedError


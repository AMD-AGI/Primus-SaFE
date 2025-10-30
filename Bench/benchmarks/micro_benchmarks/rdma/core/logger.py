import logging
import os
from datetime import datetime
from .config import config


def get_logger(name: str = None) -> logging.Logger:
    """
    Create and return a logger instance using global config.
    """
    logger_name = name or config.log_name
    logger = logging.getLogger(logger_name)
    logger.setLevel(config.log_level)

    if not logger.handlers:
        # Console handler
        ch = logging.StreamHandler()
        ch.setLevel(config.log_level)
        ch_formatter = logging.Formatter(
            "[%(asctime)s][%(levelname)s][%(name)s] %(message)s",
            "%Y-%m-%d %H:%M:%S"
        )
        ch.setFormatter(ch_formatter)
        logger.addHandler(ch)

        # File handler
        if config.log_dir:
            os.makedirs(config.log_dir, exist_ok=True)
            log_file = os.path.join(
                config.log_dir,
                f"{logger_name}_{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
            )
            fh = logging.FileHandler(log_file)
            fh.setLevel(config.log_level)
            fh.setFormatter(ch_formatter)
            logger.addHandler(fh)

    return logger

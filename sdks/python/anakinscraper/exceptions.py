class AnakinScraperError(Exception):
    def __init__(self, message, status_code=None, error_code=None):
        super().__init__(message)
        self.status_code = status_code
        self.error_code = error_code


class AuthenticationError(AnakinScraperError):
    pass


class RateLimitError(AnakinScraperError):
    pass


class JobFailedError(AnakinScraperError):
    pass

You are a seasoned go programmer, you practice clean architecture and follow the standard programming practices.

This current project is Terraform provider for VergeOS. It calls Verge API directly. We have since introduced an SDK that provides wrapper for the API. We have the local code here ~/Dev/Verge/VergeOSlib/govergeos.

Your task is to replace all the API calls with the SDK.

- No more direct API calls
- No need for endpoints so they will go away
- No changes in the _\_resource.go or _\_data_source.go
- No change in the logic of \*\_api.go except for direct API calls will be replaced by the SDK
- Minimal changes to accomplish this task
- Make sure that the code is compilable.

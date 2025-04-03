## **Contributing & Issue Guidelines**

Thank you for considering contributing to this project! To ensure smooth collaboration, please follow these guidelines when creating or addressing GitHub issues.

### **How to Report a Bug**

If you encounter a bug, please create a **new issue** using the **Bug Report** template:

1. Navigate to the **Issues** tab.
2. Click **New Issue** → Select **Bug Report**.
3. Provide the following details:
   - **Description**: A clear explanation of the issue.
   - **Steps to Reproduce**: How to reproduce the bug.
   - **Expected vs. Actual Behavior**.
   - **Screenshots/Logs (if applicable)**.
   - **Environment details** (OS, version, dependencies, etc.).
4. Submit the issue and assign appropriate **labels** (`bug`, `high priority`, etc.).

### **Requesting a Feature**

To suggest a new feature:

1. Click **New Issue** → Select **Feature Request**.
2. Provide:
   - **Problem Statement**: What problem does this feature solve?
   - **Proposed Solution**: Your idea for implementation.
   - **Potential Alternatives** (if any).
   - **Additional Context** (mockups, references, etc.).
3. Assign relevant **labels** (`enhancement`, `good first issue`, etc.).

### **Contributing to an Issue**

If you'd like to work on an issue:

1. **Check Open Issues**: Look for issues labeled **"help wanted"** or **"good first issue"**.
2. **Comment on the Issue**: Mention that you're taking it (e.g., _"I'd like to work on this"_).
3. **Fork the Repository & Clone it**:

   ```sh
   git clone https://github.com/ametow/xpos.git
   ```

4. **Create a Branch**:

   ```sh
   git checkout -b feature-branch-name
   ```

5. **Commit & Push**:

   ```sh
   git commit -m "Describe changes"
   git push -u origin feature-branch-name
   ```

6. **Open a Pull Request**:
   - Navigate to the repository on GitHub.
   - Click **New Pull Request**.
   - Select your branch and submit it for review.

### **Issue Labels & Status**

| Label | Description |
|--------|------------|
| `bug` | A confirmed bug in the codebase. |
| `enhancement` | A request for a new feature or improvement. |
| `help wanted` | Issues that need contributions. |
| `good first issue` | Beginner-friendly issues to get started with. |
| `in progress` | Someone is actively working on this. |
| `duplicate` | The issue already exists. |

### Pull Request (PR) Guidelines  

When submitting a PR, please follow these guidelines to ensure smooth review and integration:  

1. **Follow the Branch Naming Convention**:  
   - `feature/feature-name` (for new features)  
   - `bugfix/bug-description` (for bug fixes)  
   - `hotfix/urgent-fix` (for critical fixes)  

2. **Keep PRs Focused & Small**:  
   - Solve only one issue per PR.  
   - If the PR grows too large, consider breaking it down.  

3. **Write Clear Commit Messages**:  
   - Use present-tense: _"Fix memory leak in API"_  
   - Keep it under 50 characters for the first line.  
   - Add details if necessary in a second paragraph.  

4. **Ensure Code Quality**:  
   - Run tests before submitting (`make test`, `go test ./...`, etc.).  
   - Follow the project’s coding standards.  
   - Check for linting errors (`golangci-lint run`, `eslint`, `prettier`, etc.).  

5. **Provide a Descriptive PR Title & Summary**:  
   - Clearly state what the PR does and why it’s needed.  
   - Reference related issues (`Closes #123`).  

6. **Request a Review**:  
   - Assign a reviewer if applicable.  
   - Address all feedback before merging.  

7. **Check PR Labels & Status**  

   | Label | Description |  
   |--------|------------|  
   | `needs review` | Awaiting review from maintainers. |  
   | `changes requested` | Requires updates before merging. |  
   | `ready to merge` | Approved and ready for deployment. |  

### **Code of Conduct**

- Be respectful and constructive.
- Keep discussions relevant and on-topic.
- Report any abusive behavior.

# 🌌 NootyCLI v0.2.2 Radin Pro

### ⚡ نخستین کد ایجنت هوشمند، بومی و ضدتحریم ترمینال در ایران
#### The First Iranian Anti-Sanction Agentic Terminal Intelligence

🌐 **[وب‌سایت رسمی (cli.nooty.ir)](https://cli.nooty.ir)** | 📧 **پشتیبانی:** support@nooty.ir | info@nooty.ir

---

![NootyCLI Banner](https://img.shields.io/badge/NootyCLI-v0.2.2_Radin_Pro-6f42c1?style=for-the-badge&logo=gnu-bash&logoColor=white)
![Go Version](https://img.shields.io/badge/Go-1.22%2B_Pure-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/Platform-macOS_%7C_Linux-black?style=for-the-badge&logo=apple&logoColor=white)
![DNS Shield](https://img.shields.io/badge/DNS_Shield-Electro_%7C_Shecan_%7C_Begzar-green?style=for-the-badge&logo=shield&logoColor=white)

---

## 🎯 هدف پروژه

ابزار **NootyCLI** با هدف ارائه یک **دستیار کدنویسی خودکار و عاملیت‌محور (Code Agent)** ویژه برنامه‌نویسان و توسعه‌دهندگان ایرانی خلق شده است. این ابزار بدون نیاز به ابزارهای تغییر آی‌پی یا وابستگی‌های سنگین، ترمینال شما را به یک محیط هوشمند تبدیل می‌کند تا بتوانید ساختار پروژه‌ها را تحلیل، فایل‌ها را ویرایش و دستورات شل را با الگوی **Plan & Execute** به صورت کاملاً خودکار و هوشمند اجرا کنید. 🚀✨

---

## ✨ قابلیت‌های برجسته

- 🧠 **موتور عاملیت هوشمند (Agentic Plan & Execute):** تفکیک برنامه‌ریزی و اجرا؛ ارائه نقشه‌راه شفاف قبل از اجرای هر دستور و دریافت تاییدیه از کاربر.
- 🛡️ **سپر ضد تحریم بومی (Zero-Config DNS Shield):** زنجیره خودکار Fallback روی دی‌ان‌اس‌های Electro ، Shecan و Begzar در صورت قطع ارتباط یا خطاهای ۴۰۳.
- 🛠️ **۱۱ ابزار بومی ترمینال (Built-in Tools):** پیمایش درختی پروژه‌ها، جستجوی پیشرفته کد، ویرایش خط‌به‌خط فایل‌ها، خروجی Git و اجرای امن دستورات شل با مدیریت Timeout.
- 📦 **صفر وابستگی خارجی (Zero Dependencies):** توسعه داده‌شده ۱۰۰٪ با کتابخانه استاندارد زبان Go جهت بالا بردن سرعت، امنیت و سهولت کامپایل.
- 🏥 **سیستم عیب‌یابی پزشک (/doctor):** دستور اختصاصی برای بررسی زنده وضعیت اتصال شبکه، سلامت کلید API و لیست مدل‌های فعال.

---

## 🚀 راهنمای نصب سریع

کافیست دستور تک‌خطی زیر را در ترمینال مک‌بوک (macOS) یا لینوکس (Linux) خود وارد کنید:
```bash
curl -fsSL "https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/install.sh?v=$(date +%s)" | bash
```

> ⚡ **نکته:** اسکریپت نصب به صورت خودکار معماری پردازنده شما (arm64 یا amd64) را تشخیص داده و فایل اجرایی را در مسیر `/usr/local/bin/nooty` قرار می‌دهد.

---

## 💻 نحوه استفاده

### ۱. اجرای ابزار
```bash
nooty
```

### ۲. فعال‌سازی حالت عاملیتی (Agent Mode)
برای اینکه ایجنت اجازه دسترسی به فایل‌ها و اجرای دستورات پروژه را داشته باشد:
```bash
/mode cli
```

### ۳. ارسال دستور کار به ایجنت
```text
ساختار پروژه را بررسی کن، یک فایل test.go بساز، توش Hello World بنویس و اجراش کن.
```

---

## ⚙️ جدول دستورات و حالت‌ها

| دستور | نحوه عملکرد |
| :--- | :--- |
| `/mode cli` | سوییچ به حالت عاملیتی (امکان اجرای ابزارها، ویرایش فایل و اجرای دستورات) |
| `/mode chat` | سوییچ به حالت چت معمولی (پرسش و پاسخ متنی بدون دسترسی به فایل‌ها) |
| `/doctor` | اجرای عیب‌یابی زنده سلامت شبکه، DNS Shield و دسترسی به API |
| `/model list` | نمایش لیست مدل‌های هوش مصنوعی موجود با قابلیت انتخاب |
| `/clear` | پاک‌سازی محیط متنی ترمینال |
| `/help` | نمایش راهنما و دستورات کمکی |

---

## 📬 ثبت بازخورد و گزارش خطا

این نسخه (v0.2.2 Radin Pro) در حال حاضر به صورت عرضه محدود (Beta) منتشر شده است.

- 📧 **ایمیل پشتیبانی:** support@nooty.ir | info@nooty.ir
- 🌐 **وب‌سایت رسمی:** [cli.nooty.ir](https://cli.nooty.ir)
- 🐛 **ثبت باگ در گیت‌هاب:** [GitHub Issues](https://github.com/parsaprz429/Nooty/issues)

---

## 🇬🇧 English Overview

**NootyCLI** is the first Iranian anti-sanction **Code Agent** built with Pure Go. It transforms your terminal into an agentic AI workspace using a **Plan & Execute** system to read, write, and execute shell commands safely.

### ⚡ Quick One-Line Install
```bash
curl -fsSL "https://raw.githubusercontent.com/parsaprz429/Nooty/main/NootyCLI/install.sh?v=$(date +%s)" | bash
```

- **Official Website:** [cli.nooty.ir](https://cli.nooty.ir)
- **Support:** support@nooty.ir

---

<p align="center">
  Powered by <b>Nooty Ecosystem</b> 🚀
</p>
